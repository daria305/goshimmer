package main

import (
	"encoding/csv"
	"fmt"
	"github.com/iotaledger/goshimmer/packages/tangle"
	"github.com/iotaledger/goshimmer/tools/experiment/logger"
	"github.com/iotaledger/goshimmer/tools/experiment/utils"
	"math"
	"sync"
	"time"
)

const (
	MaxParentAge         = time.Minute
	K                    = 2
	Mps                  = 50
	AttackDuration       = 8
	MeasurementsInterval = MaxParentAge / 4
	IdleSpamTime         = MaxParentAge * 1
	IdleHonestRate       = 2
)

var (
	urls = []string{"http://localhost:8080", "http://localhost:8090", "http://localhost:8060", "http://localhost:8050", "http://localhost:8040", "http://localhost:8030", "http://localhost:8020"}

	adversaryUrl = []string{"http://localhost:8070"}

	log = logger.New("orphanage")
)

func main() {
	qParams := createQs(K, 0.1, 0.1, 1)
	RunOrphanageExperiment(K, Mps, AttackDuration, MaxParentAge, qParams)
}

func createQs(k int, start, step, stop float64) []float64 {
	criticalVal := 1 - (1 / float64(k))
	log.Infof("Critical value expected: %f for K=%d", criticalVal, k)
	fracCriticalVal := make([]float64, 0)
	for v := start; math.Round(v*100)/100 < stop; v += step {
		fracCriticalVal = append(fracCriticalVal, math.Round(v*100)/100)
	}
	fracCriticalVal = append(fracCriticalVal, criticalVal)
	n := len(fracCriticalVal)
	qs := make([]float64, n)
	for i := 0; i < n-1; i++ {
		qs[i] = fracCriticalVal[i] * criticalVal
	}
	qs[n-1] = criticalVal

	log.Infof("q parameters calculated: %v", qs)
	return qs
}

type ExperimentParams struct {
	ExpId                int
	MaxParentAge         time.Duration
	K                    int
	Q                    float64
	Mps                  int
	AttackDuration       int         // attack duration = n * MaxParentAge
	MeasureTimes         []time.Time // cutoff = cutoffs[i] * MaxParentsAge
	MeasurementsInterval time.Duration
	IdleSpamTime         time.Duration // honest activity messages spam duration before and after an attack
	IdleHonestRate       int
	AdversaryID          string
	StartTime            time.Time // start time of an attack
	StopTime             time.Time // stop time of an attack
}

func RunOrphanageExperiment(k, mps, duration int, maxParentAge time.Duration, qRange []float64) {

	fileName := fmt.Sprintf("orphanage-maxAge_%ds-k_%d-%s.csv", int(MaxParentAge.Seconds()), K, time.Now().UTC().Format(time.RFC3339))
	csvWriter := createWriter(fileName, header)

	honestClts := utils.NewClients(urls, "honest")
	adversaryClts := utils.NewClients(adversaryUrl, "adversary")

	walkStartMessageID := tangle.EmptyMessageID
	for expId := 0; expId < len(qRange); expId++ {
		params := &ExperimentParams{
			ExpId:                expId,
			MaxParentAge:         maxParentAge,
			K:                    k,
			Q:                    qRange[expId],
			Mps:                  mps,
			AttackDuration:       duration,
			MeasurementsInterval: MeasurementsInterval,
			IdleSpamTime:         IdleSpamTime,
			IdleHonestRate:       IdleHonestRate,
		}
		runSingleExperiment(params, walkStartMessageID, csvWriter, honestClts, adversaryClts)
	}
}

func runSingleExperiment(params *ExperimentParams, startMsgID tangle.MessageID, csvWriter *csv.Writer, honestClts *utils.Clients, adversaryClts *utils.Clients) (nextStartMsg tangle.MessageID) {
	adversaryInfo, _ := adversaryClts.GetGoShimmerAPIs()[0].Info()
	params.AdversaryID = adversaryInfo.IdentityIDShort

	// determine rates
	honestRate := int(float64(params.Mps) * (1 - params.Q) / float64(len(honestClts.GetGoShimmerAPIs())))
	adversaryRate := int(float64(params.Mps) * params.Q)

	wg := &sync.WaitGroup{}

	//  START IDLE ACTIVITY MESSAGES SPAM only honest nodes
	log.Infof("Idle period for next %s, only honest activity messages, num of honest nodes: %d, rate per node: %d", params.IdleSpamTime.String(), len(honestClts.GetGoShimmerAPIs()), params.IdleHonestRate)
	honestClts.Spam(params.IdleHonestRate, params.IdleSpamTime, "unit", wg)
	wg.Wait()

	// START ORPHANAGE ATTACK
	startTime := time.Now()
	attackDuration := time.Duration(params.AttackDuration) * params.MaxParentAge

	log.Infof("Starting an orphanage attack with q=%f, mps=%d, advNodeID: %s, num of honest nodes: %d", params.Q, params.Mps, params.AdversaryID, len(honestClts.GetGoShimmerAPIs()))
	honestClts.Spam(honestRate, attackDuration, "unit", wg)
	adversaryClts.Spam(adversaryRate, attackDuration, "unit", wg)
	wg.Wait()

	stopTime := time.Now()

	// UPDATE PARAMS AFTER ATTACK FINISHED evaluated after experiment finished
	params.MeasureTimes = calculateCutoffs(startTime, stopTime, params.MeasurementsInterval)
	params.StartTime = startTime
	params.StopTime = stopTime

	log.Infof("Idle spamming started")
	honestClts.Spam(params.IdleHonestRate, MaxParentAge*2, "unit", wg)
	wg.Wait()

	for idx, node := range honestClts.GetGoShimmerAPIs() {
		// TODO make it async
		// request orphanage data
		log.Infof("Spamming has finished! Requesting orphanage data from honest nodes.")
		resp, err := node.GetDiagnosticsOrphanage(tangle.EmptyMessageID, startTime, stopTime, params.MeasureTimes)
		if err != nil {
			log.Errorf("Error: %s, %s", resp.Error, err)
			return
		}
		log.Infof("Response received from honest node nr %d", idx)
		requester := resp.CreatorNodeID
		msgId, err := tangle.NewMessageID(resp.LastMessageID)
		if err != nil {
			log.Errorf("Failed to retrieve nextMessageID: %s", err.Error())
			msgId = tangle.EmptyMessageID
		}
		nextStartMsg = msgId

		resultLines, err := ParseResults(params, resp, requester)
		if err != nil {
			log.Error(err)
		}
		log.Infof("Writing to csv file, requester %s", requester)
		err = csvWriter.WriteAll(resultLines)
		if err != nil {
			log.Errorf("Failed to write results to csv file: %s", err.Error())
			return
		}
		csvWriter.Flush()
	}

	return
}

func calculateCutoffs(startTime, stopTime time.Time, interval time.Duration) (measurePoints []time.Time) {
	for currentTime := startTime.Add(interval); currentTime.Before(stopTime); currentTime = currentTime.Add(interval) {
		measurePoints = append(measurePoints, currentTime)
	}
	return
}
