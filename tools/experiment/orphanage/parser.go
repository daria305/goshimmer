package main

import (
	"encoding/csv"
	"github.com/cockroachdb/errors"
	"github.com/iotaledger/goshimmer/packages/jsonmodels"
	"os"
	"path"
	"strconv"
)

var (
	header = []string{"expId", "q", "mps", "honestOrphanageRate", "advOrphanageRate", "totalOrphans", "honestOrphans", "advOrphans", "totalIssued", "honestIssued", "advIssued", "requester", "attackDuration", "intervalNum", "intervalStart", "intervalStop"}
)

func ParseResults(params *ExperimentParams, respData *jsonmodels.OrphanageResponse, requesterID string) ([][]string, error) {
	log.Infof("Parsing the results for requester %s", requesterID)
	honestIssued, honestOrphaned, advIssued, advOrphaned := calculateOrphanage(respData.IssuedByNode, respData.OrphansByNode, params.AdversaryID)

	// create intervals
	intervalStartTime := params.StartTime
	times := append(params.MeasureTimes, params.StopTime)

	csvLines := make([][]string, len(params.MeasureTimes)+1)

	if len(honestIssued) == 0 {
		return csvLines, errors.Errorf("Honest issued message list is empty! There are no results to parse.")
	}
	if len(advIssued) == 0 {
		return csvLines, errors.Errorf("Adversary issued message list is empty! There are no results to parse.")
	}

	for i := range csvLines {
		intervalStopTime := times[i]
		var honestRate float64
		if honestIssued[i] == 0 {
			honestRate = 0
		} else {
			honestRate = float64(honestOrphaned[i]) / float64(honestIssued[i])
		}

		var advRate float64
		if advIssued[i] == 0 || len(advIssued) == 0 {
			advRate = 0
		} else {
			advRate = float64(advOrphaned[i]) / float64(advIssued[i])
		}
		csvLine := []string{
			strconv.Itoa(params.ExpId),
			strconv.FormatFloat(params.Q, 'f', 3, 64),
			strconv.Itoa(params.Mps),
			strconv.FormatFloat(honestRate, 'f', 3, 64),
			strconv.FormatFloat(advRate, 'f', 3, 64),
			strconv.Itoa(honestOrphaned[i] + advOrphaned[i]),
			strconv.Itoa(honestOrphaned[i]),
			strconv.Itoa(advOrphaned[i]),
			strconv.Itoa(honestIssued[i] + advIssued[i]),
			strconv.Itoa(honestIssued[i]),
			strconv.Itoa(advIssued[i]),
			requesterID,
			strconv.Itoa(params.AttackDuration),
			strconv.Itoa(i + 1),
			strconv.Itoa(int(intervalStartTime.UnixMicro())),
			strconv.Itoa(int(intervalStopTime.UnixMicro())),
		}
		csvLines[i] = csvLine

		intervalStartTime = intervalStopTime
	}
	return csvLines, nil
}

func calculateOrphanage(issuedBy, orphanedBy map[string][]int, adversaryID string) ([]int, []int, []int, []int) {
	issuers := make([]string, 0)
	for issuer := range issuedBy {
		issuers = append(issuers, issuer)
	}

	// end of each time range is endTime
	// beginning of first time range is startTime, of each next time range is startTime + cutoff[i]

	log.Debugf("numberOfTimeRanges: ")
	for key := range issuedBy {
		log.Debugf("IssuedBy[%s] = %d", key, issuedBy[key])
	}
	for key := range orphanedBy {
		log.Debugf("orphanedBy[%s] = %d", key, orphanedBy[key])
	}
	numberOfTimeRanges := len(issuedBy[adversaryID]) // num of startCutoff+1
	honestIssued := make([]int, numberOfTimeRanges)
	advIssued := make([]int, numberOfTimeRanges)
	honestOrphaned := make([]int, numberOfTimeRanges)
	advOrphaned := make([]int, numberOfTimeRanges)

	for _, issuer := range issuers {
		if issuer == adversaryID {
			countMessagesBy(issuedBy, orphanedBy, advIssued, advOrphaned, issuer)
		} else {
			countMessagesBy(issuedBy, orphanedBy, honestIssued, honestOrphaned, issuer)
		}
	}
	return honestIssued, honestOrphaned, advIssued, advOrphaned
}

func countMessagesBy(issuedBy, orphanedBy map[string][]int, issued, orphaned []int, issuer string) {
	countMsgBy(issuedBy, issued, issuer)
	countMsgBy(orphanedBy, orphaned, issuer)
}

func countMsgBy(issuedBy map[string][]int, issued []int, issuer string) {
	for i, countPerRange := range issuedBy[issuer] {
		if len(issued) == 0 {
			log.Debugf("len of issued == 0, for issuer: %s", issuer)
			return
		}
		issued[i] += countPerRange
	}
}

func createWriter(dirPath string, fileName string, header []string) *csv.Writer {
	file, err := os.Create(path.Join(dirPath, fileName))
	if err != nil {
		panic(err)
	}
	resultsWriter := csv.NewWriter(file)

	// Write the headers
	if err := resultsWriter.Write(header); err != nil {
		panic(err)
	}
	return resultsWriter
}
