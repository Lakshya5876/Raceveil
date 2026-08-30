package oracle

import "testing"

func TestClassify(t *testing.T) {
	success := &Rule{Status: 200}
	reject := &Rule{Status: 409}

	cases := []struct {
		name string
		resp ObservedResponse
		want Classification
	}{
		{"success status", ObservedResponse{StatusCode: 200}, Success},
		{"reject status", ObservedResponse{StatusCode: 409}, Reject},
		{"neither", ObservedResponse{StatusCode: 500}, Unclassified},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.resp, success, reject); got != c.want {
				t.Errorf("Classify(%+v) = %v, want %v", c.resp, got, c.want)
			}
		})
	}
}

func TestClassify_BodyContains(t *testing.T) {
	success := &Rule{BodyContains: "redeemed"}
	if got := Classify(ObservedResponse{Body: `{"status":"redeemed"}`}, success, nil); got != Success {
		t.Errorf("expected body-matched success, got %v", got)
	}
	if got := Classify(ObservedResponse{Body: `{"status":"other"}`}, success, nil); got != Unclassified {
		t.Errorf("expected non-match to stay unclassified, got %v", got)
	}
}

func TestCountSuccesses(t *testing.T) {
	success := &Rule{Status: 200}
	reject := &Rule{Status: 409}
	responses := []ObservedResponse{
		{StatusCode: 200}, {StatusCode: 200}, {StatusCode: 409}, {StatusCode: 200},
	}
	if got := CountSuccesses(responses, success, reject); got != 3 {
		t.Errorf("CountSuccesses = %d, want 3", got)
	}
}
