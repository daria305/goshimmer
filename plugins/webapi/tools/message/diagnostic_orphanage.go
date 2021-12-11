package message

import (
	"github.com/iotaledger/goshimmer/packages/consensus/gof"
	"github.com/iotaledger/goshimmer/packages/jsonmodels"
	"github.com/iotaledger/goshimmer/packages/tangle"
	"github.com/iotaledger/hive.go/datastructure/walker"
	"github.com/iotaledger/hive.go/identity"
	"github.com/labstack/echo"
	"net/http"
	"time"
)

func DiagnosticOrphanageHandler(c echo.Context) error {
	return diagnosticOrphanage(c)
}

func diagnosticOrphanage(c echo.Context) error {
	startMsgID, startTime, stopTime, middlePoints, err := readDiagnosticOrphanageRequest(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, jsonmodels.OrphanageResponse{Error: err.Error()})
	}
	ownId := deps.Local.ID().String()
	// to measure at once two ranges, and see if rate stabilizes a while after spam starts
	orphanCounts := make(map[string][]int)
	issuedCounts := make(map[string][]int)
	var lastMsgID tangle.MessageID
	var lastMsgTimestamp time.Time

	maxAge := deps.Tangle.Options.SolidifierParams.MaxParentsTimeDifference

	deps.Tangle.Utils.WalkMessageID(func(msgID tangle.MessageID, walker *walker.Walker) {
		// we assume no conflicts
		approverMessageIDs := deps.Tangle.Utils.ApprovingMessageIDs(msgID)
		var timestamp time.Time
		var issuerID string

		deps.Tangle.Storage.Message(msgID).Consume(func(message *tangle.Message) {
			timestamp = message.IssuingTime()
			pubKey := message.IssuerPublicKey()
			issuerID = identity.New(pubKey).ID().String()
		})
		currentStartTime := startTime

		measurePoints := append(middlePoints, stopTime)
		measurementsNumber := len(measurePoints)
		if timestamp.After(startTime) && timestamp.Before(stopTime) {
			createInnerMapIfFirstSeen(issuedCounts, orphanCounts, issuerID, measurementsNumber)
			lastMsgTimestamp, timestamp, lastMsgID, msgID = updateLastMessageID(lastMsgTimestamp, timestamp, lastMsgID, msgID)
			// count messages within time intervals between two consecutive measurement points
			for intervalNum, currentEndTime := range measurePoints {
				if timestamp.After(currentStartTime) && timestamp.Before(currentEndTime) {
					increaseInnerCount(issuedCounts, issuerID, intervalNum)
					if len(approverMessageIDs) == 0 {
						increaseInnerCount(orphanCounts, issuerID, intervalNum)
					}
					// should be added only to one interval at a time
					break
				}
			}
		}
		// continue walking
		for _, approverMessageID := range approverMessageIDs {
			walker.Push(approverMessageID)
		}
	}, tangle.MessageIDs{startMsgID}, false)

	return c.JSON(http.StatusOK, jsonmodels.NewOrphanageResponse(ownId, maxAge, lastMsgID, orphanCounts, issuedCounts))
}

func increaseInnerCount(cuntsMap map[string][]int, key string, innerSliceIdx int) {
	if _, ok := cuntsMap[key]; !ok {
		cuntsMap[key] = make([]int, 2)
	}
	cuntsMap[key][innerSliceIdx]++
}

func createInnerMapIfFirstSeen(issuerCounts, orphanageCounts map[string][]int, key string, measurementsNumber int) {
	if _, ok := issuerCounts[key]; !ok {
		issuerCounts[key] = make([]int, measurementsNumber)
		orphanageCounts[key] = make([]int, measurementsNumber)
	}
}

func updateLastMessageID(lastMsgTimestamp, timestamp time.Time, lastMsgID, msgID tangle.MessageID) (time.Time, time.Time, tangle.MessageID, tangle.MessageID) {
	if lastMsgTimestamp.Before(timestamp) {
		deps.Tangle.Storage.MessageMetadata(msgID).Consume(func(messageMetadata *tangle.MessageMetadata) {
			// last message is updated only if it is confirmed, so it could be used as a starting point of next API call starting messageID
			if messageMetadata.GradeOfFinality() == gof.High {
				lastMsgTimestamp = timestamp
				lastMsgID = msgID
			}

		})
	}
	return lastMsgTimestamp, timestamp, lastMsgID, msgID
}

func readDiagnosticOrphanageRequest(c echo.Context) (startMsgID tangle.MessageID, startTime, stopTime time.Time, measurePoints []time.Time, err error) {
	var request jsonmodels.OrphanageRequest
	if err = c.Bind(&request); err != nil {
		return
	}
	if request.StartMsgID == "" {
		startMsgID = tangle.EmptyMessageID
	} else {
		if startMsgID, err = tangle.NewMessageID(request.StartMsgID); err != nil {
			return
		}
	}
	startTime = time.UnixMicro(request.StartTime)
	if stopTime = time.UnixMicro(request.StopTime); stopTime.UnixMicro() == 0 {
		stopTime = time.Now()
	}

	for _, point := range request.MeasurePoints {
		measurePoints = append(measurePoints, time.UnixMicro(point))
	}
	return
}

type DiagnosticOrphanage struct {
	orphansCount   int
	orphanedMsgIDs tangle.MessageIDs
}
