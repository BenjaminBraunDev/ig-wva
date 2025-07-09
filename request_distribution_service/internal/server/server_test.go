package server

import (
	"context"
	"testing"

	rd_go_proto "ig-wva/gen/go/request_distribution"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestNewCoreLogicHandler(t *testing.T) {
	h := NewCoreLogicHandler()
	if h == nil {
		t.Fatal("NewCoreLogicHandler() returned nil")
	}
	if h.currentConfig != nil {
		t.Error("Expected h.currentConfig to be nil on init")
	}
	if len(h.currentRequestTypes) != 0 {
		t.Errorf("Expected h.currentRequestTypes to be empty, got %d", len(h.currentRequestTypes))
	}
	if len(h.currentDistribution) != 0 {
		t.Errorf("Expected h.currentDistribution to be empty, got %d", len(h.currentDistribution))
	}
}

func TestGetPowerOfTwoBucket(t *testing.T) {
	testCases := []struct {
		name           string
		tokenCount     int
		expectedBucket string
	}{
		{"zero tokens", 0, "0-1"},
		{"one token", 1, "0-1"},
		{"two tokens", 2, "2-3"},
		{"three tokens", 3, "2-3"},
		{"four tokens", 4, "4-7"},
		{"seven tokens", 7, "4-7"},
		{"eight tokens", 8, "8-15"},
		{"15 tokens", 15, "8-15"},
		{"16 tokens", 16, "16-31"},
		{"1023 tokens", 1023, "512-1023"},
		{"1024 tokens", 1024, "1024-2047"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bucket := getPowerOfTwoBucket(tc.tokenCount)
			if bucket != tc.expectedBucket {
				t.Errorf("getPowerOfTwoBucket(%d) = %s; want %s", tc.tokenCount, bucket, tc.expectedBucket)
			}
		})
	}
}

func TestUpdateDatasetAndRates_Success(t *testing.T) {
	h := NewCoreLogicHandler()
	ctx := context.Background()

	// Helper variable to get a pointer for the optional field.
	maxSamples := int32(3)
	req := &rd_go_proto.UpdateDatasetAndRatesRequest{
		DatasetRequests: []*rd_go_proto.DatasetRequest{
			{
				DatasetName:      "test-dataset-1",
				TokenizerName:    "test-tokenizer-1",
				InputColumn:      "input-1",
				OutputColumn:     "output-1",
				TotalRequestRate: 60.0,
				LatencySloTpotMs: 50.0,
				MaxSamples:       &maxSamples,
			},
			{
				DatasetName:      "test-dataset-2",
				TokenizerName:    "test-tokenizer-2",
				InputColumn:      "input-2",
				OutputColumn:     "output-2",
				TotalRequestRate: 40.0,
				LatencySloTpotMs: 50.0,
				MaxSamples:       &maxSamples,
			},
		},
	}

	resp, err := h.UpdateDatasetAndRates(ctx, req)
	if err != nil {
		t.Fatalf("UpdateDatasetAndRates() failed: %v", err)
	}
	if resp == nil || resp.GetMessage() == "" {
		t.Error("UpdateDatasetAndRates() response message is empty")
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Verify that the currentConfig is updated
	if diff := cmp.Diff(req, h.currentConfig, protocmp.Transform()); diff != "" {
		t.Errorf("currentConfig mismatch (-want +got):\n%s", diff)
	}
}

func TestGetCurrentDistribution_Success(t *testing.T) {
	h := NewCoreLogicHandler()
	ctx := context.Background()

	// Helper variable for the optional field.
	maxSamples := int32(2)
	// First, load some data
	updateReq := &rd_go_proto.UpdateDatasetAndRatesRequest{
		DatasetRequests: []*rd_go_proto.DatasetRequest{
			{
				DatasetName:      "main-dataset-1",
				TokenizerName:    "main-tokenizer-1",
				InputColumn:      "in_col-1",
				OutputColumn:     "out_col-1",
				TotalRequestRate: 150.0,
				LatencySloTpotMs: 75.0,
				MaxSamples:       &maxSamples,
			},
		},
	}

	_, err := h.UpdateDatasetAndRates(ctx, updateReq)
	if err != nil {
		t.Fatalf("Setup for GetCurrentDistribution failed: %v", err)
	}

	// Now, get the distribution
	getResp, err := h.GetCurrentDistribution(ctx, &rd_go_proto.GetCurrentDistributionRequest{})
	if err != nil {
		t.Fatalf("GetCurrentDistribution() failed: %v", err)
	}

	if diff := cmp.Diff(updateReq.GetDatasetRequests(), getResp.GetSourceDatasetRequests(), protocmp.Transform()); diff != "" {
		t.Errorf("GetCurrentDistribution source requests mismatch (-want +got):\n%s", diff)
	}
}
