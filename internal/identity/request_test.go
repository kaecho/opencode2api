package identity

import (
	"strings"
	"testing"
	"time"
)

func TestStableProjectID(t *testing.T) {
	a := StableProjectID("server-key")
	b := StableProjectID("server-key")
	if a != b {
		t.Fatalf("project id is not stable: %q != %q", a, b)
	}
	if len(a) != 40 || strings.Trim(a, "0123456789abcdef") != "" {
		t.Fatalf("project id is not 40 lowercase hex: %q", a)
	}
	if StableProjectID("other-key") == a {
		t.Fatal("different scopes share one project id")
	}
}

func TestScopedSessionStable(t *testing.T) {
	a := scopedSessionID("scope-a", "conversation-a")
	b := scopedSessionID("scope-a", "conversation-a")
	if a != b {
		t.Fatalf("session id is not stable: %q != %q", a, b)
	}
	if !isOfficialSessionID(a) {
		t.Fatalf("derived session has the wrong shape: %q", a)
	}
	if scopedSessionID("scope-b", "conversation-a") == a {
		t.Fatal("different scopes share one session id")
	}
}

func TestOfficialSessionVerbatim(t *testing.T) {
	official := "ses_e81ddbba8ffe47froHx1zYPehv"
	if got := scopedSessionID("scope", official); got != official {
		t.Fatalf("official session was rewritten: %q", got)
	}
}

func TestProjectForKey(t *testing.T) {
	now := time.Date(2026, 10, 28, 1, 2, 3, 0, time.UTC)
	a := ProjectForKey("base-project", "key-a", now)
	if a != ProjectForKey("base-project", "key-a", now) {
		t.Fatal("project id is not stable for one key and day")
	}
	if len(a) != 40 || strings.Trim(a, "0123456789abcdef") != "" {
		t.Fatalf("project id is not 40 lowercase hex: %q", a)
	}
	if ProjectForKey("base-project", "key-b", now) == a {
		t.Fatal("different keys share a project id")
	}
	if ProjectForKey("base-project", "key-a", now.Add(24*time.Hour)) == a {
		t.Fatal("project id did not rotate with the UTC day")
	}
}

func TestSessionForKey(t *testing.T) {
	a := SessionForKey("ses_e81ddbba8ffe47froHx1zYPehv", "key-a")
	if a != SessionForKey("ses_e81ddbba8ffe47froHx1zYPehv", "key-a") {
		t.Fatal("session id is not stable for one key and conversation")
	}
	if !isOfficialSessionID(a) {
		t.Fatalf("per-key session has the wrong shape: %q", a)
	}
	if SessionForKey("ses_e81ddbba8ffe47froHx1zYPehv", "key-b") == a {
		t.Fatal("different keys share a session id")
	}
}

func TestForUpstreamKeyIsolatesIdentity(t *testing.T) {
	now := time.Date(2026, 10, 28, 1, 2, 3, 0, time.UTC)
	ids := RequestIDs{
		Session:       "ses_e81ddbba8ffe47froHx1zYPehv",
		ChildSession:  "ses_e81ddbba8ffe47froHx1zYPEhv",
		ParentSession: "ses_e81ddbba8ffe47froHx1zYPehx",
		Project:       "470c8549ef01a2a9d3c85b22965766cefc874134",
		Request:       "req_1234",
		Message:       "msg_1234",
	}
	a := ForUpstreamKey(ids, "key-a", now)
	b := ForUpstreamKey(ids, "key-a", now)
	c := ForUpstreamKey(ids, "key-b", now)
	if a.Project != b.Project || a.Session != b.Session || a.ChildSession != b.ChildSession || a.ParentSession != b.ParentSession {
		t.Fatal("per-key identity is not stable")
	}
	if a.Project == c.Project || a.Session == c.Session || a.ChildSession == c.ChildSession || a.ParentSession == c.ParentSession {
		t.Fatal("per-key identity was shared across keys")
	}
	if a.Request != ids.Request || a.Message != ids.Message {
		t.Fatal("request or message id changed")
	}
}
