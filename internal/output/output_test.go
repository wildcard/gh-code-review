package output

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestStableEnvelopeAndExitCodes(t *testing.T) {
	var buffer bytes.Buffer
	exit := WriteSuccess(&buffer, "validate", "o/r", 3, map[string]bool{"valid": true})
	if exit != ExitSuccess {
		t.Fatalf("exit=%d", exit)
	}
	var envelope Envelope
	if err := json.Unmarshal(buffer.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != "1.0" || !envelope.OK || envelope.Operation != "validate" {
		t.Fatalf("bad envelope: %#v", envelope)
	}

	buffer.Reset()
	exit = WriteError(&buffer, "submit", "o/r", 3, NewError(ExitHeadChanged, "HEAD_CHANGED", "changed", true, nil))
	if exit != ExitHeadChanged {
		t.Fatalf("exit=%d", exit)
	}
	if err := json.Unmarshal(buffer.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error == nil || envelope.Error.Code != "HEAD_CHANGED" || !envelope.Error.Retryable {
		t.Fatalf("bad error envelope: %#v", envelope)
	}
}

func TestExitCodeContract(t *testing.T) {
	got := []int{
		ExitSuccess, ExitInternal, ExitValidation, ExitHeadChanged, ExitAuth,
		ExitAPI, ExitPartial, ExitUnsupported, ExitConflict,
	}
	for expected, actual := range got {
		if actual != expected {
			t.Fatalf("exit code index %d is %d", expected, actual)
		}
	}
}
