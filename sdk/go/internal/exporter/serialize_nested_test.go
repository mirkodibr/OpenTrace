package exporter

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/opentrace/opentrace-go/internal/wire"
)

// TestSerializeNestedFields verifies Object/StringSlice/Map fields survive
// serialisation as genuine nested JSON, including deep recursion.
func TestSerializeNestedFields(t *testing.T) {
	e := testEvent("nested event",
		wire.String("action", "checkout"),
		wire.Object("user",
			wire.String("id", "u_123"),
			wire.String("email", "u@example.com"),
			wire.Object("permissions",
				wire.Bool("billing", true),
				wire.Bool("admin", false),
			),
		),
		wire.Object("http",
			wire.String("method", "POST"),
			wire.Int("status", 200),
		),
		wire.StringSlice("tags", []string{"vip", "beta"}),
		wire.Map("labels", map[string]string{"tier": "gold"}),
	)

	var buf bytes.Buffer
	err := serialize(&buf, []*wire.LogEvent{e}, testResource())
	wire.ReleaseEvent(e)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var payload struct {
		Events []struct {
			LogAttributes map[string]interface{} `json:"log_attributes"`
		} `json:"events"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	attrs := payload.Events[0].LogAttributes

	user, ok := attrs["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("user is not a nested object: %T", attrs["user"])
	}
	if user["id"] != "u_123" {
		t.Errorf("user.id = %v", user["id"])
	}
	perms, ok := user["permissions"].(map[string]interface{})
	if !ok {
		t.Fatalf("user.permissions is not nested: %T", user["permissions"])
	}
	if perms["billing"] != true || perms["admin"] != false {
		t.Errorf("permissions = %v", perms)
	}

	httpObj := attrs["http"].(map[string]interface{})
	if httpObj["method"] != "POST" || httpObj["status"] != 200.0 {
		t.Errorf("http = %v", httpObj)
	}

	tags, ok := attrs["tags"].([]interface{})
	if !ok || len(tags) != 2 || tags[0] != "vip" {
		t.Errorf("tags = %v", attrs["tags"])
	}

	labels, ok := attrs["labels"].(map[string]interface{})
	if !ok || labels["tier"] != "gold" {
		t.Errorf("labels = %v", attrs["labels"])
	}
}
