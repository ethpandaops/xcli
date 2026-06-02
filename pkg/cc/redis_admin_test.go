package cc

import "testing"

const (
	testKeyCBTExternal123 = "cbt:external:123"
	testPrefixCBT         = "cbt:"
)

func TestSplitKeyByPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		prefix       string
		key          string
		wantBranch   string
		wantLeaf     string
		wantAccepted bool
	}{
		{
			name:         "root branch",
			prefix:       "",
			key:          testKeyCBTExternal123,
			wantBranch:   testPrefixCBT,
			wantAccepted: true,
		},
		{
			name:         "root leaf",
			prefix:       "",
			key:          "singleton",
			wantLeaf:     "singleton",
			wantAccepted: true,
		},
		{
			name:         "nested branch",
			prefix:       testPrefixCBT,
			key:          testKeyCBTExternal123,
			wantBranch:   "cbt:external:",
			wantAccepted: true,
		},
		{
			name:         "nested leaf",
			prefix:       "cbt:external:",
			key:          testKeyCBTExternal123,
			wantLeaf:     testKeyCBTExternal123,
			wantAccepted: true,
		},
		{
			name:         "non matching prefix",
			prefix:       testPrefixCBT,
			key:          "lab:foo",
			wantAccepted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotBranch, gotLeaf, ok := splitKeyByPrefix(tt.prefix, tt.key)
			if ok != tt.wantAccepted {
				t.Fatalf("accepted mismatch: got %t want %t", ok, tt.wantAccepted)
			}

			if gotBranch != tt.wantBranch {
				t.Fatalf("branch mismatch: got %q want %q", gotBranch, tt.wantBranch)
			}

			if gotLeaf != tt.wantLeaf {
				t.Fatalf("leaf mismatch: got %q want %q", gotLeaf, tt.wantLeaf)
			}
		})
	}
}

func TestBuildVersionTokenSetOrderInsensitive(t *testing.T) {
	t.Parallel()

	base := redisKeyDetailResponse{
		Type:  redisTypeSet,
		TTLMS: 1234,
		SetMembers: []redisEncodedValue{
			{Mode: keyText, Text: "a"},
			{Mode: keyText, Text: "b"},
		},
	}

	reordered := redisKeyDetailResponse{
		Type:  redisTypeSet,
		TTLMS: 1234,
		SetMembers: []redisEncodedValue{
			{Mode: keyText, Text: "b"},
			{Mode: keyText, Text: "a"},
		},
	}

	if buildVersionToken(base) != buildVersionToken(reordered) {
		t.Fatalf("set version token should be order-insensitive")
	}
}

func TestBuildVersionTokenListOrderSensitive(t *testing.T) {
	t.Parallel()

	first := redisKeyDetailResponse{
		Type:  "list",
		TTLMS: 1,
		ListItems: []redisEncodedValue{
			{Mode: keyText, Text: "a"},
			{Mode: keyText, Text: "b"},
		},
	}

	second := redisKeyDetailResponse{
		Type:  "list",
		TTLMS: 1,
		ListItems: []redisEncodedValue{
			{Mode: keyText, Text: "b"},
			{Mode: keyText, Text: "a"},
		},
	}

	if buildVersionToken(first) == buildVersionToken(second) {
		t.Fatalf("list version token should be order-sensitive")
	}
}
