package jsonmodels

import (
	"github.com/iotaledger/goshimmer/packages/tangle"
	"time"
)

// PastconeRequest holds the message id to query.
type PastconeRequest struct {
	ID string `json:"id"`
}

// PastconeResponse is the HTTP response containing the number of messages in the past cone and if all messages of the past cone
// exist on the node.
type PastconeResponse struct {
	Exist        bool   `json:"exist,omitempty"`
	PastConeSize int    `json:"pastConeSize,omitempty"`
	Error        string `json:"error,omitempty"`
}

// MissingResponse is the HTTP response containing all the missing messages and their count.
type MissingResponse struct {
	IDs   []string `json:"ids,omitempty"`
	Count int      `json:"count,omitempty"`
}

// MissingAvailableResponse is a map of messageIDs with the peers that have such message.
type MissingAvailableResponse struct {
	Availability map[string][]string `json:"msgavailability,omitempty"`
	Count        int                 `json:"count"`
}

// OrphanageRequest is a struct providing request for an orphanage diagnostic API.
type OrphanageRequest struct {
	StartMsgID  string `json:"startMsgID,omitempty"`
	StartTime   int64  `json:"startTime,omitempty"`
	StopTime    int64  `json:"stopTime,omitempty"`
	CutoffStart int64  `json:"cutoffStart,omitempty"`
}

// NewOrphanageRequest creates a request object for OrphanageResponse json model.
func NewOrphanageRequest(startMsgID tangle.MessageID, startTime, stopTime time.Time, cutStart time.Duration) *OrphanageRequest {
	return &OrphanageRequest{
		StartMsgID:  startMsgID.Base58(),
		StartTime:   startTime.UnixMicro(),
		StopTime:    stopTime.UnixMicro(),
		CutoffStart: cutStart.Microseconds(),
	}
}

// OrphanageResponse is a struct providing response for an orphanage diagnostic API.
type OrphanageResponse struct {
	Error         string           `json:"error,omitempty"`
	CreatorNodeID string           `json:"creatorNodeId"`
	MaxParentAge  int64            `json:"maxParentAge"`
	OrphansByNode map[string][]int `json:"orphansByNode"`
	IssuedByNode  map[string][]int `json:"issuedByNode"`
	LastMessageID string           `json:"lastMessageID"`
}

// NewOrphanageResponse creates a response object for OrphanageResponse json model.
func NewOrphanageResponse(nodeId string, maxAge time.Duration, lastMsgID tangle.MessageID, orphansByNode, issuedByNode map[string][]int) *OrphanageResponse {
	return &OrphanageResponse{
		CreatorNodeID: nodeId,
		MaxParentAge:  maxAge.Microseconds(),
		OrphansByNode: orphansByNode,
		IssuedByNode:  issuedByNode,
		LastMessageID: lastMsgID.Base58(),
	}
}
