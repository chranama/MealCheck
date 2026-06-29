package checker

import "testing"

func TestCandidateAssistValidationAcceptsKnownCandidate(t *testing.T) {
	request := BuildCandidateAssistRequest("oats", "cup", "breakfast", []CandidateAssistCandidate{
		{CandidateID: "fndds:1", Name: "Oatmeal, cooked"},
	})
	decision, err := DecodeAndValidateCandidateAssistResponse(`{"action":"select_candidate","candidate_id":"fndds:1","confidence":"high","reason":"same food"}`, request)
	if err != nil {
		t.Fatalf("DecodeAndValidateCandidateAssistResponse error: %v", err)
	}
	if decision.Action != CandidateAssistActionSelect || decision.Candidate.CandidateID != "fndds:1" {
		t.Fatalf("decision = %+v, want selected known candidate", decision)
	}
}

func TestCandidateAssistValidationRejectsInventedCandidate(t *testing.T) {
	request := BuildCandidateAssistRequest("oats", "cup", "breakfast", []CandidateAssistCandidate{
		{CandidateID: "fndds:1", Name: "Oatmeal, cooked"},
	})
	_, err := DecodeAndValidateCandidateAssistResponse(`{"action":"select_candidate","candidate_id":"fndds:999","confidence":"high","reason":"invented"}`, request)
	if err == nil {
		t.Fatal("DecodeAndValidateCandidateAssistResponse error = nil, want invented candidate rejection")
	}
}

func TestCandidateAssistValidationAllowsAbstention(t *testing.T) {
	request := BuildCandidateAssistRequest("mystery food", "", "", []CandidateAssistCandidate{
		{CandidateID: "fndds:1", Name: "Oatmeal, cooked"},
	})
	decision, err := DecodeAndValidateCandidateAssistResponse(`{"action":"no_safe_match","candidate_id":"","confidence":"medium","reason":"candidate list does not cover the item"}`, request)
	if err != nil {
		t.Fatalf("DecodeAndValidateCandidateAssistResponse error: %v", err)
	}
	if decision.Action != CandidateAssistActionNoMatch || decision.Candidate.CandidateID != "" {
		t.Fatalf("decision = %+v, want no safe match", decision)
	}
}
