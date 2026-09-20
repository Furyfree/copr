package pins

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPackagedFileRoundTrip(t *testing.T) {
	onDisk, err := os.ReadFile("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Decode(onDisk)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(onDisk) {
		t.Fatalf("pins.json is not the encoded form:\n%s", encoded)
	}
	for _, name := range []string{Copilot, Toolbox, Wowup} {
		data, err := Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		var a Artifact
		if err := json.Unmarshal(data, &a); err != nil || a.Version == "" || a.SHA256 == "" {
			t.Fatalf("%s: %+v %v", name, a, err)
		}
	}
}

func TestReplaceOneHelper(t *testing.T) {
	data, err := Lookup(Copilot)
	if err != nil {
		t.Fatal(err)
	}
	var a Artifact
	if err := json.Unmarshal(data, &a); err != nil {
		t.Fatal(err)
	}
	a.Version = "1.2.3"
	a.Source = "https://github.com/github/app/releases/download/v1.2.3/GitHub-Copilot-linux-x64.rpm"
	replaced, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := Replace(packed, Copilot, replaced)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LookupIn(updated, Copilot)
	if err != nil {
		t.Fatal(err)
	}
	var out Artifact
	if err := json.Unmarshal(got, &out); err != nil || out.Version != "1.2.3" {
		t.Fatalf("%+v %v", out, err)
	}
	wowup, err := LookupIn(updated, Wowup)
	if err != nil {
		t.Fatal(err)
	}
	original, err := Lookup(Wowup)
	if err != nil {
		t.Fatal(err)
	}
	if string(wowup) != string(original) {
		t.Fatal("unrelated helper pin changed")
	}
}

func TestDecodeRejectsMalformedPins(t *testing.T) {
	for _, data := range []string{`{}`, `{"github-copilot-installer":{}}{}`, `{"other":{}}`} {
		if _, err := Decode([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	if _, err := Lookup("nimbus"); err == nil || !strings.Contains(err.Error(), "unknown installer helper") {
		t.Fatalf("unknown helper: %v", err)
	}
}
