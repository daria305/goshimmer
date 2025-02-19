package client

import (
	"encoding/csv"
	"fmt"
	"github.com/iotaledger/goshimmer/packages/jsonmodels"
	"github.com/iotaledger/goshimmer/packages/tangle"
	"net/http"
	"time"
)

const (
	routeDiagnostics = "tools/diagnostic"
	// RouteDiagnosticMessages is the API route for message diagnostics.
	RouteDiagnosticMessages = routeDiagnostics + "/messages"
	// RouteDiagnosticsFirstWeakMessageReferences is the API route for first weak message diagnostics.
	RouteDiagnosticsFirstWeakMessageReferences = RouteDiagnosticMessages + "/firstweakreferences"
	// RouteDiagnosticsMessageRank is the API route for message diagnostics with a rank filter.
	RouteDiagnosticsMessageRank = RouteDiagnosticMessages + "/rank/:rank"
	// RouteDiagnosticsUtxoDag is the API route for Utxo Dag diagnostics.
	RouteDiagnosticsUtxoDag = routeDiagnostics + "/utxodag"
	// RouteDiagnosticsBranches is the API route for branches diagnostics.
	RouteDiagnosticsBranches = routeDiagnostics + "/branches"
	// RouteDiagnosticsLazyBookedBranches is the API route for booked branches diagnostics.
	RouteDiagnosticsLazyBookedBranches = RouteDiagnosticsBranches + "/lazybooked"
	// RouteDiagnosticsInvalidBranches is the API route for invalid branches diagnostics.
	RouteDiagnosticsInvalidBranches = RouteDiagnosticsBranches + "/invalid"
	// RouteDiagnosticsTips is the API route for tips diagnostics.
	RouteDiagnosticsTips = routeDiagnostics + "/tips"
	// RouteDiagnosticsDRNG is the API route for DRNG diagnostics.
	RouteDiagnosticsDRNG = routeDiagnostics + "/drng"
	// RouteDiagnosticsOrphanage is the APi route for orphanage diagnostic with end time in unix nano format.
	RouteDiagnosticsOrphanage = routeDiagnostics + "/orphanage"
)

// GetDiagnosticsMessages runs full message diagnostics
// Returns CSV with the following fields:
//
//	ID IssuerID IssuerPublicKey IssuanceTime ArrivalTime SolidTime ScheduledTime BookedTime GradeOfFinality
//	GradeOfFinalityTime StrongParents WeakParents DislikeParents LikeParents StrongApprovers WeakApprovers BranchID InclusionState Scheduled ScheduledBypass Booked
//	Invalid Finalized Rank IsPastMarker PastMarkers PMHI PMLI FutureMarkers FMHI FMLI PayloadType TransactionID
func (api *GoShimmerAPI) GetDiagnosticsMessages() (*csv.Reader, error) {
	return api.diagnose(RouteDiagnosticMessages)
}

// GetDiagnosticsFirstWeakMessageReferences runs diagnostics over weak references only.
// Returns CSV with the following fields:
//
//	ID IssuerID IssuerPublicKey IssuanceTime ArrivalTime SolidTime ScheduledTime BookedTime GradeOfFinality
//	GradeOfFinalityTime StrongParents WeakParents DislikeParents LikeParents StrongApprovers WeakApprovers BranchID InclusionState Scheduled Booked
//  Invalid Finalized Rank IsPastMarker PastMarkers PMHI PMLI FutureMarkers FMHI FMLI PayloadType TransactionID
func (api *GoShimmerAPI) GetDiagnosticsFirstWeakMessageReferences() (*csv.Reader, error) {
	return api.diagnose(RouteDiagnosticsFirstWeakMessageReferences)
}

// GetDiagnosticsMessagesByRank run diagnostics for messages whose markers are equal or above a certain rank
// Returns CSV with the following fields:
//
//	ID IssuerID IssuerPublicKey IssuanceTime ArrivalTime SolidTime ScheduledTime BookedTime GradeOfFinality
//	GradeOfFinalityTime StrongParents WeakParents DislikeParents LikeParents StrongApprovers WeakApprovers BranchID InclusionState Scheduled Booked
//	Invalid Finalized Rank IsPastMarker PastMarkers PMHI PMLI FutureMarkers FMHI FMLI PayloadType TransactionID
func (api *GoShimmerAPI) GetDiagnosticsMessagesByRank(rank uint64) (*csv.Reader, error) {
	return api.diagnose(fmt.Sprintf("%s?rank=%d", RouteDiagnosticMessages, rank))
}

// GetDiagnosticsUtxoDag runs diagnostics over utxo dag.
// Returns csv with the following fields:
//
//	ID,IssuanceTime,SolidTime,AccessManaPledgeID,ConsensusManaPledgeID,Inputs,Outputs,Attachments,
//	BranchID,Conflicting,LazyBooked,GradeOfFinality,GradeOfFinalityTime
func (api *GoShimmerAPI) GetDiagnosticsUtxoDag() (*csv.Reader, error) {
	return api.diagnose(RouteDiagnosticsUtxoDag)
}

// GetDiagnosticsBranches runs diagnostics over branches.
// Returns csv with the following fields:
//
//	ID,ConflictSet,IssuanceTime,SolidTime,LazyBooked,GradeOfFinality
func (api *GoShimmerAPI) GetDiagnosticsBranches() (*csv.Reader, error) {
	return api.diagnose(RouteDiagnosticsBranches)
}

// GetDiagnosticsLazyBookedBranches runs diagnostics over lazy booked branches.
// Returns csv with the following fields:
//
//	ID,ConflictSet,IssuanceTime,SolidTime,LazyBooked,GradeOfFinality
func (api *GoShimmerAPI) GetDiagnosticsLazyBookedBranches() (*csv.Reader, error) {
	return api.diagnose(RouteDiagnosticsLazyBookedBranches)
}

// GetDiagnosticsInvalidBranches runs diagnostics over invalid branches.
// Returns csv with the following fields:
//
//	ID,ConflictSet,IssuanceTime,SolidTime,LazyBooked,GradeOfFinality
func (api *GoShimmerAPI) GetDiagnosticsInvalidBranches() (*csv.Reader, error) {
	return api.diagnose(RouteDiagnosticsInvalidBranches)
}

// GetDiagnosticsTips runs diagnostics over tips
// Returns csv with the following fields:
//
//	tipType ID IssuerID IssuerPublicKey IssuanceTime ArrivalTime SolidTime ScheduledTime BookedTime GradeOfFinality
//	GradeOfFinalityTime StrongParents WeakParents DislikeParents LikeParents StrongApprovers WeakApprovers BranchID InclusionState Scheduled ScheduledBypass Booked
//	Invalid Finalized Rank IsPastMarker PastMarkers PMHI PMLI FutureMarkers FMHI FMLI PayloadType TransactionID
func (api *GoShimmerAPI) GetDiagnosticsTips() (*csv.Reader, error) {
	return api.diagnose(RouteDiagnosticsTips)
}

// GetDiagnosticsDRNG runs diagnostics for DRNG
// Returns csv with the following fields:
//
// 	ID,IssuerID,IssuerPublicKey,IssuanceTime,ArrivalTime,SolidTime,ScheduledTime,BookedTime,
//	dRNGPayloadType,InstanceID,Round,PreviousSignature,Signature,DistributedPK
func (api *GoShimmerAPI) GetDiagnosticsDRNG() (*csv.Reader, error) {
	return api.diagnose(RouteDiagnosticsDRNG)
}

// GetDiagnosticsOrphanage runds diagnostic orphanage over the tangle, takes current time that will indicate
// time after which it can stop walking, to syncronize results from different clients
// Returns OrphanageResponse json response.
func (api *GoShimmerAPI) GetDiagnosticsOrphanage(startMsgID tangle.MessageID, startTime, stopTime time.Time, measurePoints []time.Time) (*jsonmodels.OrphanageResponse, error) {
	request := jsonmodels.NewOrphanageRequest(startMsgID, startTime, stopTime, measurePoints)
	response := &jsonmodels.OrphanageResponse{}

	route := RouteDiagnosticsOrphanage

	if err := api.do(http.MethodGet, route, request, response); err != nil {
		return nil, err
	}
	return response, nil
}

// run an api call on a certain route and return a csv.
func (api *GoShimmerAPI) diagnose(route string) (*csv.Reader, error) {
	reader := &csv.Reader{}
	if err := api.do(http.MethodGet, route, nil, reader); err != nil {
		return nil, err
	}
	return reader, nil
}
