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
	startMsgID, startTime, stopTime, cutoffStart, err := readDiagnosticOrphanageRequest(c)
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
		cutoffStop := stopTime.Add(-maxAge)
		var timestamp time.Time
		var issuer string

		deps.Tangle.Storage.Message(msgID).Consume(func(message *tangle.Message) {
			timestamp = message.IssuingTime()
			pubKey := message.IssuerPublicKey()
			issuer = identity.New(pubKey).ID().String()
		})
		// wider range from startTime to cutoffStop
		if timestamp.After(startTime) && timestamp.Before(cutoffStop) {
			lastMsgTimestamp, timestamp, lastMsgID, msgID = updateLastMessageID(lastMsgTimestamp, timestamp, lastMsgID, msgID)

			increaseInnerCount(issuedCounts, issuer, 0)
			// message has no parents - is orphaned
			if len(approverMessageIDs) == 0 {
				increaseInnerCount(orphanCounts, issuer, 0)
			}
			// inner range from startTime+cutoff to cutoffStop
			if timestamp.After(startTime.Add(cutoffStart)) {
				increaseInnerCount(issuedCounts, issuer, 1)
				if len(approverMessageIDs) == 0 {
					increaseInnerCount(orphanCounts, issuer, 1)
				}
			}
		}
		// continue walking
		for _, approverMessageID := range approverMessageIDs {
			walker.Push(approverMessageID)
		}
	}, tangle.MessageIDs{startMsgID})

	return c.JSON(http.StatusOK, jsonmodels.NewOrphanageResponse(ownId, maxAge, lastMsgID, issuedCounts, orphanCounts))
}

func increaseInnerCount(cuntsMap map[string][]int, key string, innerSliceIdx int) {
	if _, ok := cuntsMap[key]; !ok {
		cuntsMap[key] = make([]int, 2)
	}
	cuntsMap[key][innerSliceIdx]++
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

func readDiagnosticOrphanageRequest(c echo.Context) (startMsgID tangle.MessageID, startTime, stopTime time.Time, cutStart time.Duration, err error) {
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
	cutStart = time.Duration(request.CutoffStart) * time.Microsecond
	return
}

type DiagnosticOrphanage struct {
	orphansCount   int
	orphanedMsgIDs tangle.MessageIDs
}
