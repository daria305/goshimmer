package main

import (
	"encoding/csv"
	"fmt"
	"github.com/iotaledger/goshimmer/client"
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
	urls         = []string{"http://localhost:8080", "http://localhost:8090", "http://localhost:8060", "http://localhost:8050", "http://localhost:8040"}
	adversaryUrl = []string{"http://localhost:8070"}

	log = logger.New("orphanage")
)

func main() {
	qParams := createQs(K, 0.1, 0.1, 1)
	RunOrphanageExperiment(K, Mps, AttackDuration, MaxParentAge, qParams)

	// IdleSpamToRecoverTheNetwork(time.Minute, 10)
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

func NewExperimentParams(k int, mps int, duration int, maxParentAge time.Duration, qRange []float64, expId int) *ExperimentParams {
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
	return params
}

func RunOrphanageExperiment(k, mps, duration int, maxParentAge time.Duration, qRange []float64) {

	fileName := fmt.Sprintf("orphanage-maxAge_%ds-k_%d-%s.csv", int(MaxParentAge.Seconds()), K, time.Now().Format("020106_030405PM"))
	csvWriter := createWriter(fileName, header)
	defer csvWriter.Flush()

	log.Info("Experiment will take: %s", time.Duration(len(qRange))*MaxParentAge+time.Duration(2+len(qRange)-1)*IdleSpamTime)

	honestClts := utils.NewClients(urls, "honest")
	adversaryClts := utils.NewClients(adversaryUrl, "adversary")
	grafanaLinks := make([]string, 0)

	// list Grafana dashboard links in log at the end of the experiment
	defer func() {
		for i, link := range grafanaLinks {
			log.Infof("Experiment %d: %s", i, link)
		}
	}()

	expStart := time.Now()
	walkStartMessageID := tangle.EmptyMessageID
	for expId := 0; expId < len(qRange); expId++ {
		log.Infof("Experiment nr %d has started.", expId)
		params := NewExperimentParams(k, mps, duration, maxParentAge, qRange, expId)

		if !utils.IsNetworkAlive(honestClts, adversaryClts) {
			log.Infof("Experiment finished , the network is down after %s", time.Since(expStart).String())
			break
		}
		_, link := runSingleExperiment(params, walkStartMessageID, csvWriter, honestClts, adversaryClts)
		grafanaLinks = append(grafanaLinks, link)
		log.Infof("Experiment finished %d: %s", expId, link)
	}
	log.Infof("Grafana link to all experiments: %s", createGrafanaLinkForExperimentDuration(expStart, time.Now()))
}

func IdleSpamToRecoverTheNetwork(duration time.Duration, rate int) {
	honestClts := utils.NewClients(urls, "honest")
	wg := &sync.WaitGroup{}

	//  START IDLE ACTIVITY MESSAGES SPAM only honest nodes
	log.Infof("Idle period for next %s, only honest activity messages, num of honest nodes: %d, rate per node: %d", duration.String(), len(honestClts.GetGoShimmerAPIs()), rate)
	honestClts.Spam(rate, duration, "unit", wg)
	wg.Wait()
}

func runSingleExperiment(params *ExperimentParams, startMsgID tangle.MessageID, csvWriter *csv.Writer, honestClts *utils.Clients, adversaryClts *utils.Clients) (nextStartMsg tangle.MessageID, grafanaLink string) {
	adversaryInfo, _ := adversaryClts.GetGoShimmerAPIs()[0].Info()
	params.AdversaryID = adversaryInfo.IdentityIDShort
	honestRate, adversaryRate := calculateRates(params, honestClts)
	wg := &sync.WaitGroup{}

	idleSpam(params, honestClts, wg)

	// START ORPHANAGE ATTACK
	performOrphanageAttack(params, honestClts, honestRate, wg, adversaryClts, adversaryRate)

	idleSpam(params, honestClts, wg)

	grafanaLink = createGrafanaLinkForExperimentDuration(params.StartTime, params.StopTime)

	apis := honestClts.GetGoShimmerAPIs()
	csvMutex := sync.Mutex{}
	resChan := make(chan [][]string, len(apis))
	for idx, node := range apis {
		go responseAndParseResults(node, idx, params, resChan)
	}

	// awaiting results of an experiment to be collected
	select {
	case resp := <-resChan:
		if resp != nil {
			func() {
				csvMutex.Lock()
				defer csvMutex.Unlock() // read the requester id from the first row of data
				requesterID := resp[0][11]
				log.Infof("Writing to csv file, requester %s", requesterID)
				err := csvWriter.WriteAll(resp)
				if err != nil {
					log.Errorf("Failed to write results to csv file: %s", err.Error())
					return
				}
				csvWriter.Flush()
			}()
		}
	case <-time.After(ResponseTimeout):
		log.Infof("Response not received in time")
	}

	return
}

func performOrphanageAttack(params *ExperimentParams, honestClts *utils.Clients, honestRate int, wg *sync.WaitGroup, adversaryClts *utils.Clients, adversaryRate int) {
	startTime := time.Now()
	attackDuration := time.Duration(params.AttackDuration) * params.MaxParentAge

	log.Infof("Starting an orphanage attack with q=%f, mps=%d, advNodeID: %s, num of honest nodes: %d", params.Q, params.Mps, params.AdversaryID, len(honestClts.GetGoShimmerAPIs()))
	honestClts.Spam(honestRate, attackDuration, "unit", wg)
	adversaryClts.Spam(adversaryRate, attackDuration, "unit", wg)
	wg.Wait()

	stopTime := time.Now()
	updateParamsAfterExpFinishes(params, startTime, stopTime)
}

func idleSpam(params *ExperimentParams, honestClts *utils.Clients, wg *sync.WaitGroup) {
	log.Infof("Idle period for next %s, only honest activity messages, num of honest nodes: %d, rate per node: %d", params.IdleSpamTime.String(), len(honestClts.GetGoShimmerAPIs()), params.IdleHonestRate)
	honestClts.Spam(params.IdleHonestRate, params.IdleSpamTime, "unit", wg)
	log.Info("idle spam finishes")
	wg.Wait()
}

func calculateRates(params *ExperimentParams, honestClts *utils.Clients) (int, int) {
	honestRate := int(float64(params.Mps) * (1 - params.Q) / float64(len(honestClts.GetGoShimmerAPIs())))
	adversaryRate := int(float64(params.Mps) * params.Q)
	return honestRate, adversaryRate
}

func updateParamsAfterExpFinishes(params *ExperimentParams, startTime time.Time, stopTime time.Time) {
	params.MeasureTimes = calculateCutoffs(startTime, stopTime, params.MeasurementsInterval)
	params.StartTime = startTime
	params.StopTime = stopTime
}

func calculateCutoffs(startTime, stopTime time.Time, interval time.Duration) (measurePoints []time.Time) {
	for currentTime := startTime.Add(interval); currentTime.Before(stopTime); currentTime = currentTime.Add(interval) {
		measurePoints = append(measurePoints, currentTime)
	}
	return
}

func createGrafanaLinkForExperimentDuration(startTime, stopTime time.Time) string {
	return fmt.Sprintf("Graphana: http://localhost:3000/d/B7yT2rhnz/goshimmer-debugging?orgId=1&from=%v000&to=%v000&inspect=80&inspectTab=data", startTime.Unix(), stopTime.Unix())
}

func responseAndParseResults(node *client.GoShimmerAPI, nodeIndex int, params *ExperimentParams, respChan chan<- [][]string) {
	// request orphanage data
	log.Infof("Spamming has finished! Requesting orphanage data from honest nodes.")
	diagnosticStart := time.Now()
	resp, err := node.GetDiagnosticsOrphanage(tangle.EmptyMessageID, params.StartTime, params.StopTime, params.MeasureTimes)
	if err != nil {
		log.Error(err)
		respChan <- nil
		return
	}
	log.Infof("Response received from honest node nr %d, after %s", nodeIndex, time.Since(diagnosticStart).String())
	requester := resp.CreatorNodeID
	_, err = tangle.NewMessageID(resp.LastMessageID)
	if err != nil {
		log.Errorf("Failed to retrieve nextMessageID: %s", err.Error())
		//msgId = tangle.EmptyMessageID
	}
	//nextStartMsg = msgId

	resultLines, err := ParseResults(params, resp, requester)
	if err != nil {
		log.Error(err)
		respChan <- nil
		return
	}
	respChan <- resultLines
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
