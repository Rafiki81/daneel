package approval_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/approval"
)

func req(tool string) daneel.ApprovalRequest {
	return daneel.ApprovalRequest{
		Agent:     "test-agent",
		Tool:      tool,
		Args:      json.RawMessage(`{"key":"value"}`),
		SessionID: "sess-1",
	}
}

func TestAutoApprove(t *testing.T) {
	a := approval.AutoApprove()
	ok, err := a.Approve(context.Background(), req("any"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Fatal("AutoApprove should approve")
	}
}

func TestAlwaysDeny(t *testing.T) {
	a := approval.AlwaysDeny()
	ok, err := a.Approve(context.Background(), req("any"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("AlwaysDeny should deny")
	}
}

func TestDenyWithReason(t *testing.T) {
	a := approval.DenyWithReason("not allowed")
	ok, err := a.Approve(context.Background(), req("exec"))
	if ok {
		t.Fatal("should deny")
	}
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err = %v, want containing not allowed", err)
	}
}

func TestCallback(t *testing.T) {
	var capturedTool string
	a := approval.Callback(func(agent, tool string, args map[string]any) bool {
		capturedTool = tool
		return tool == "safe"
	})

	ok, _ := a.Approve(context.Background(), req("safe"))
	if !ok {
		t.Fatal("safe should be approved")
	}
	if capturedTool != "safe" {
		t.Fatalf("captured tool = %q", capturedTool)
	}

	ok, _ = a.Approve(context.Background(), req("danger"))
	if ok {
		t.Fatal("danger should be denied")
	}
}

func TestPolicyFirstMatchWins(t *testing.T) {
	p := approval.NewPolicy().
		Deny("dangerous").
		Allow("dangerous"). // should not matter, first match wins
		Allow("safe")

	ok, _ := p.Approve(context.Background(), req("dangerous"))
	if ok {
		t.Fatal("dangerous should be denied (first match)")
	}

	ok, _ = p.Approve(context.Background(), req("safe"))
	if !ok {
		t.Fatal("safe should be allowed")
	}
}

func TestPolicyUnmatchedDenied(t *testing.T) {
	p := approval.NewPolicy().Allow("known")
	ok, _ := p.Approve(context.Background(), req("unknown"))
	if ok {
		t.Fatal("unmatched tool should be denied")
	}
}

func TestWithLogging(t *testing.T) {
	var logged []string
	inner := approval.AutoApprove()
	a := approval.WithLogging(inner, func(tool string, approved bool) {
		logged = append(logged, tool)
	})

	a.Approve(context.Background(), req("tool1"))
	a.Approve(context.Background(), req("tool2"))

	if len(logged) != 2 {
		t.Fatalf("logged %d, want 2", len(logged))
	}
	if logged[0] != "tool1" || logged[1] != "tool2" {
		t.Fatalf("logged = %v", logged)
	}
}

func TestWithTimeoutExpires(t *testing.T) {
	// Inner approver that blocks forever
	inner := daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	})
	a := approval.WithTimeout(inner, 50*time.Millisecond)
	ok, err := a.Approve(context.Background(), req("slow"))
	if ok {
		t.Fatal("should deny on timeout")
	}
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
}

func TestWithTimeoutFastApproval(t *testing.T) {
	inner := approval.AutoApprove()
	a := approval.WithTimeout(inner, 5*time.Second)
	ok, err := a.Approve(context.Background(), req("fast"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Fatal("should approve quickly")
	}
}

func TestConsoleWithWriterApproves(t *testing.T) {
	var buf bytes.Buffer
	reader := strings.NewReader("y\n")
	a := approval.ConsoleWithWriter(&buf, reader)
	ok, err := a.Approve(context.Background(), req("mytool"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Fatal("should approve on y")
	}
	output := buf.String()
	if !strings.Contains(output, "mytool") {
		t.Fatal("should display tool name")
	}
	if !strings.Contains(output, "Approval Required") {
		t.Fatal("should show approval prompt")
	}
}

func TestConsoleWithWriterDenies(t *testing.T) {
	var buf bytes.Buffer
	reader := strings.NewReader("n\n")
	a := approval.ConsoleWithWriter(&buf, reader)
	ok, err := a.Approve(context.Background(), req("tool"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("should deny on n")
	}
}
