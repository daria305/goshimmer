package main

import (
	"fmt"
	"github.com/iotaledger/goshimmer/packages/tangle"
	"github.com/iotaledger/goshimmer/tools/experiment/logger"
	"github.com/iotaledger/goshimmer/tools/experiment/utils"
	"sync"
	"time"
)

const (
	MaxParentAge = time.Minute
	K            = 2
)

var (
	urls = []string{"http://localhost:8080", "http://localhost:8090", "http://192.168.160.9:8080", "http://192.168.160.7:8080", "http://192.168.160.8:8080"}

	adversaryUrl = []string{"http://localhost:8070"}

	log = logger.New("orphanage")
)

func main() {
	testOrphanageAPI()
}

type experimentParams struct {
	ExpId          int
	MaxParentAge   time.Duration
	K              int
	Q              float64
	Mps            int
	AttackDuration int   // attack duration = n * MaxParentAge
	Cutoffs        []int // cutoff = cutoffs[i] * MaxParentsAge
	AdversaryID    string
}

func testOrphanageAPI() {

	fileName := fmt.Sprintf("orphanage-maxAge_%ds-k_%d-%s", int(MaxParentAge.Seconds()), K, time.Now().UTC().Format(time.RFC3339))
	csvWriter := createWriter(fileName, header)

	honestClts := utils.NewClients(urls, "honest")
	adversaryClts := utils.NewClients(adversaryUrl, "adversary")

	adversaryInfo, _ := adversaryClts.GetGoShimmerAPIs()[0].Info()
	adversaryID := adversaryInfo.IdentityIDShort

	params := &experimentParams{
		ExpId:          0,
		MaxParentAge:   MaxParentAge,
		K:              K,
		Q:              0.5,
		Mps:            50,
		AttackDuration: 2,
		AdversaryID:    adversaryID,
	}
	startCutoffTime := MaxParentAge * 1
	noAdversarySpamTime := MaxParentAge * 0

	honestRate := int(float64(params.Mps) * (1 - params.Q) / float64(len(honestClts.GetGoShimmerAPIs())))
	adversaryRate := int(float64(params.Mps) * params.Q)
	idleHonestRate := 2

	// only honest messages
	log.Infof("Idle period for next %s, no malicious behaviour in the network, honest spam rate: %d", noAdversarySpamTime.String(), idleHonestRate)
	wg := &sync.WaitGroup{}
	honestClts.Spam(idleHonestRate, noAdversarySpamTime, "unit", wg)
	wg.Wait()

	// attack starts
	log.Info("Starting an orphanage attack with q=%d and mps=%d", params.Q, params.Mps)
	startTime := time.Now()
	attackDuration := time.Duration(params.AttackDuration) * params.MaxParentAge
	honestClts.Spam(honestRate, attackDuration, "unit", wg)
	adversaryClts.Spam(adversaryRate, attackDuration, "unit", wg)
	wg.Wait()

	startMsg := tangle.EmptyMessageID
	stopTime := time.Now()

	log.Infof("Spamming has finished! Requesting orphanage data from honest nodes.")
	// TODO make it async
	// request orphanage data
	resp, err := honestClts.GetGoShimmerAPIs()[0].GetDiagnosticsOrphanage(startMsg, startTime, stopTime, startCutoffTime)
	if err != nil {
		log.Errorf("Error: %s, %s", resp.Error, err)
	}

	params.Cutoffs = calculateCutoffs(params.AttackDuration)
	requester := resp.CreatorNodeID
	//nextStartMsg := resp.LastMessageID

	resultLines := ParseResults(params, resp, requester)
	err = csvWriter.WriteAll(resultLines)
	if err != nil {
		log.Errorf("Failed to write results to csv file: %s", err.Error())
		return
	}
	csvWriter.Flush()
}

func calculateCutoffs(attackDuration int) (cutoffs []int) {
	for i := 0; i < attackDuration; i++ {
		cutoffs = append(cutoffs, i)
		i++
	}
	return
}
