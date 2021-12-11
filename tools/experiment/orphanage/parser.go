package main

import (
	"encoding/csv"
	"github.com/iotaledger/goshimmer/packages/jsonmodels"
	"os"
	"path"
	"strconv"
)

var (
	header     = []string{"expId", "q", "mps", "orphanageRate", "totalOrphans", "honestOrphans", "advOrphans", "totalIssued", "honestIssued", "advIssued", "requester", "attackDuration", "startCutoffTime"}
	resultsDir = "results"
)

func ParseResults(params *experimentParams, respData *jsonmodels.OrphanageResponse, requesterID string) [][]string {

	honestIssued, honestOrphaned, advIssued, advOrphaned := calculateOrphanage(respData.IssuedByNode, respData.OrphansByNode, params.AdversaryID)

	linePerCutoff := append([]int{0}, params.Cutoffs...)
	log.Infof("linePerCutoff %d", len(linePerCutoff))
	csvLines := make([][]string, len(linePerCutoff))
	for i := range linePerCutoff {
		honestRate := honestOrphaned[i] / honestIssued[i]
		advRate := advOrphaned[i] / advIssued[i]
		csvLine := []string{
			strconv.Itoa(params.ExpId),
			strconv.FormatFloat(params.Q, 'f', 3, 64),
			strconv.Itoa(params.Mps),
			strconv.Itoa(honestRate),
			strconv.Itoa(advRate),
			strconv.Itoa(honestOrphaned[i] + advOrphaned[i]),
			strconv.Itoa(honestOrphaned[i]),
			strconv.Itoa(advOrphaned[i]),
			strconv.Itoa(honestIssued[i] + advIssued[i]),
			strconv.Itoa(honestIssued[i]),
			strconv.Itoa(advIssued[i]),
			requesterID,
			strconv.Itoa(params.AttackDuration),
			strconv.Itoa(linePerCutoff[i]),
		}
		csvLines = append(csvLines, csvLine)
	}
	return csvLines
}

func calculateOrphanage(issuedBy, orphanedBy map[string][]int, adversaryID string) ([]int, []int, []int, []int) {
	issuers := make([]string, 0)
	for issuer := range issuedBy {
		issuers = append(issuers, issuer)
	}

	// end of each time range is endTime
	// beginning of first time range is startTime, of each next time range is startTime + cutoff[i]
	numberOfTimeRanges := len(issuedBy[adversaryID]) // num of startCutoff+1
	honestIssued := make([]int, numberOfTimeRanges)
	advIssued := make([]int, numberOfTimeRanges)
	honestOrphaned := make([]int, numberOfTimeRanges)
	advOrphaned := make([]int, numberOfTimeRanges)

	for _, issuer := range issuers {
		if issuer == adversaryID {
			for i, countPerRange := range issuedBy[issuer] {
				advIssued[i] += countPerRange
			}
			for i, countPerRange := range orphanedBy[issuer] {
				advOrphaned[i] += countPerRange
			}
		} else {
			for i, countPerRange := range issuedBy[issuer] {
				honestIssued[i] += countPerRange
			}
			for i, countPerRange := range orphanedBy[issuer] {
				honestOrphaned[i] += countPerRange
			}
		}
	}
	return honestIssued, honestOrphaned, advIssued, advOrphaned
}

func createWriter(fileName string, header []string) *csv.Writer {
	// create directory for results if not exists
	if _, err := os.Stat(resultsDir); os.IsNotExist(err) {
		err = os.Mkdir(resultsDir, 0700)
		if err != nil {
			log.Error(err)
		}
	}

	file, err := os.Create(path.Join(resultsDir, fileName))
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
