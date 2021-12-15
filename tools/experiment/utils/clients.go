package utils

import (
	"encoding/csv"
	"github.com/iotaledger/goshimmer/client"
	"github.com/iotaledger/goshimmer/tools/experiment/logger"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	NetworkCheckFrequency = time.Second * 30
)

var log = logger.New("utils")

// region Clients //////////////////////////////////////////////////////////////////////////////////////////////////////

type Clients struct {
	name        string
	clients     []*client.GoShimmerAPI
	urls        []string
	deadClients []bool
	deadMu      sync.RWMutex
}

func NewClients(APIUrls []string, name string) *Clients {
	clts := make([]*client.GoShimmerAPI, len(APIUrls))
	for i, url := range APIUrls {
		clts[i] = client.NewGoShimmerAPI(url, client.WithHTTPClient(http.Client{Timeout: 10 * time.Minute}))
	}

	return &Clients{
		name:        name,
		clients:     clts,
		urls:        APIUrls[:],
		deadClients: make([]bool, len(clts)),
	}
}

func (c *Clients) GetGoShimmerAPIs() []*client.GoShimmerAPI {
	c.deadMu.RLock()
	defer c.deadMu.RUnlock()

	aliveClients := make([]*client.GoShimmerAPI, 0, len(c.clients))
	for cltNumber, cltIsAlive := range c.deadClients {
		if cltIsAlive {
			aliveClients = append(aliveClients, c.clients[cltNumber])
		}
	}
	return c.clients
}

func (c *Clients) GetClientAPIUrls() []string {
	return c.urls
}

func (c *Clients) Spam(ratePerClient int, duration time.Duration, imif string) {
	log.Infof("Spamming with %s clients and rate %d mpm has started!", c.name, ratePerClient)
	clts := c.GetGoShimmerAPIs()
	inWg := &sync.WaitGroup{}

	for cltNum := range clts {
		inWg.Add(1)
		c.toggleSpammer(ratePerClient, cltNum, imif, true)
		timer := time.NewTimer(duration)

		finishSpam := func(cltNum int) {
			c.toggleSpammer(ratePerClient, cltNum, imif, false)
			inWg.Done()
			timer.Stop()
		}
		go func(cltNum int) {
			for {
				select {
				case <-time.Tick(NetworkCheckFrequency):
					if !c.areClientsStillAlive() {
						log.Warnf("Client nr %d is dead. Stopping the spamer.", cltNum)
						finishSpam(cltNum)

						return
					}
				case <-timer.C:
					finishSpam(cltNum)
					return
				}
			}
		}(cltNum)
	}
	inWg.Wait()
}

func (c *Clients) toggleSpammer(ratePerClient int, cltNumber int, imif string, on bool) {
	spamResp, err := c.GetGoShimmerAPIs()[cltNumber].ToggleSpammer(on, ratePerClient*60, "mpm", imif)
	if err != nil {
		log.Errorf("Error: %s", err)
		return
	}
	log.Debugf("StartSpamming with %s clt nr %d: %s", c.name, cltNumber, spamResp.Message)
}

func (c *Clients) isCltAliveBySync(cltNumber int) bool {
	alive := true
	resp, err := c.GetGoShimmerAPIs()[cltNumber].Info()
	if err != nil {
		log.Errorf("Error: %s", err)
		alive = false
	} else {
		synced := resp.TangleTime.Synced
		log.Debugf("%s clt nr %d sync status: %v", c.name, cltNumber, synced)
		if !synced {
			alive = false
		}
	}
	return alive
}

func (c *Clients) isCltAliveByTips(cltNumber int) bool {
	alive := true
	resp, err := c.GetGoShimmerAPIs()[cltNumber].GetDiagnosticsTips()
	if err != nil {
		log.Errorf("Error: %s", err)
		alive = false
	} else {
		numOfLines, _ := lineCount(resp)
		log.Debugf("%s clt nr %d tip pool size: %v", c.name, cltNumber, numOfLines)
		if numOfLines == 0 {
			alive = false
		}
	}
	return alive
}

func (c *Clients) areClientsStillAlive() bool {
	c.RemoveDeadClts()

	if len(c.GetGoShimmerAPIs()) == 0 {
		return false
	}
	return true
}

func (c *Clients) RemoveDeadClts() {
	for cltNum := range c.GetGoShimmerAPIs() {
		aliveBySync := c.isCltAliveBySync(cltNum)
		aliveByTips := c.isCltAliveByTips(cltNum)
		if !aliveBySync || !aliveByTips {
			c.deadMu.Lock()
			c.deadClients[cltNum] = true
			c.deadMu.Unlock()
		}
	}
}

// endregion ////////////////////////////////////////////////////////////////////////////////////////////////////

func IsNetworkAlive(honestClts *Clients, adversaryClts *Clients) bool {
	log.Info("Checking the network status...")
	isAlive := true
	if areHonestNodesAlive := honestClts.areClientsStillAlive(); !areHonestNodesAlive {
		isAlive = false
		log.Warnf("Honest part of the network is dead. Stopping an experiment after data collection.")
	}
	if isAdversaryAlive := adversaryClts.areClientsStillAlive(); !isAdversaryAlive {
		isAlive = false
		log.Warnf("Adversary is dead. Stopping an experiment after data collection.")
	}
	return isAlive
}

// LineCount counts how many new line characters does the reader has
func lineCount(f *csv.Reader) (lineCount int, err error) {
	for {
		lineCount++
		_, err = f.Read()
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			break
		}
	}
	return
}
