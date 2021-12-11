package utils

import (
	"github.com/iotaledger/goshimmer/client"
	"github.com/iotaledger/goshimmer/tools/experiment/logger"
	"net/http"
	"sync"
	"time"
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
		clts[i] = client.NewGoShimmerAPI(url, client.WithHTTPClient(http.Client{Timeout: 30 * time.Second}))
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

func (c *Clients) Spam(ratePerClient int, duration time.Duration, imif string, wg *sync.WaitGroup) {
	log.Infof("Spamming with %s clients and rate %d mps has started!", c.name, ratePerClient)
	for cltNum := range c.GetGoShimmerAPIs() {
		wg.Add(1)
		go func(cltNum int) {
			c.toggleSpammer(ratePerClient, cltNum, imif, true)
			time.Sleep(duration)
			c.toggleSpammer(ratePerClient, cltNum, imif, false)
			wg.Done()
		}(cltNum)
	}
}

func (c *Clients) toggleSpammer(ratePerClient int, cltNumber int, imif string, on bool) {
	spamResp, err := c.GetGoShimmerAPIs()[cltNumber].ToggleSpammer(on, ratePerClient, "mps", imif)
	if err != nil {
		log.Errorf("Error: %s", err)
		return
	}
	log.Debugf("StartSpamming with %s clt nr %d: %s", c.name, cltNumber, spamResp.Message)
}

func (c *Clients) isCltAlive(cltNumber int) bool {
	alive := true
	resp, err := c.GetGoShimmerAPIs()[cltNumber].Info()
	if err != nil {
		log.Errorf("Error: %s", err)
		alive = false
	} else {
		synced := resp.TangleTime.Synced
		log.Infof("%s clt nr %d sync status: %v", c.name, cltNumber, synced)
		if !synced {
			alive = false
		}
	}
	return alive
}

func (c *Clients) RemoveDeadClts() {
	for cltNum := range c.GetGoShimmerAPIs() {
		if alive := c.isCltAlive(cltNum); !alive {
			c.deadMu.Lock()
			c.deadClients[cltNum] = true
			c.deadMu.Unlock()
		}
	}
}

// endregion ////////////////////////////////////////////////////////////////////////////////////////////////////
