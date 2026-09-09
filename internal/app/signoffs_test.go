package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v63/github"
	owners "github.com/multimediallc/codeowners-plus/internal/config"
	"github.com/multimediallc/codeowners-plus/internal/git"
	gh "github.com/multimediallc/codeowners-plus/internal/github"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

func (m *mockGitHubClient) GetCurrentOwnerSignoffs() ([]*gh.CurrentApproval, error) {
	return nil, nil
}

type signoffClient struct {
	mockGitHubClient
	signoffs      []*gh.CurrentApproval
	signoffsError error
	approved      bool
	dismissed     []*gh.CurrentApproval
}

func (m *signoffClient) GetCurrentOwnerSignoffs() ([]*gh.CurrentApproval, error) {
	return m.signoffs, m.signoffsError
}

func (m *signoffClient) FindUserApproval(user string) (*gh.CurrentApproval, error) {
	return nil, nil
}

func (m *signoffClient) ApprovePR() error {
	m.approved = true
	return nil
}

func (m *signoffClient) DismissStaleReviews(approvals []*gh.CurrentApproval) error {
	m.dismissed = append(m.dismissed, approvals...)
	return nil
}

func (m *signoffClient) CheckApprovals(fileReviewers map[string][]string, approvals []*gh.CurrentApproval, diff git.Diff) ([]codeowners.Slug, []*gh.CurrentApproval) {
	return gh.NewClient("org", "repo", "").CheckApprovals(fileReviewers, approvals, diff)
}

func newSignoffApp(t *testing.T, client *signoffClient, files []string) *App {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".codeowners"), []byte("* @org/api @fallback\n&* @org/core\nother.go @other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	diff := mockGitDiff{changes: files}
	co, err := codeowners.New(root, diff.AllChanges(), nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return &App{
		config: &Config{Quiet: true, InfoBuffer: io.Discard, WarningBuffer: io.Discard},
		client: client, codeowners: co, gitDiff: diff,
		Conf: &owners.Config{
			DisableSmartDismissal: true,
			Enforcement:           &owners.Enforcement{Approval: true, FailCheck: true},
		},
	}
}

func TestOwnerSignoffsSatisfyOwnershipOnly(t *testing.T) {
	tt := []struct {
		name              string
		reviewers         []string
		files             []string
		ordinaryApprovals []*gh.CurrentApproval
		minReviews        *int
		maxReviews        *int
		expectedSuccess   bool
		expectedApproval  bool
	}{
		{name: "all authorized AND/OR groups", reviewers: []string{"@org/api", "@org/core"}, files: []string{"api.go"}, expectedSuccess: true},
		{name: "outsider cannot sign off", files: []string{"api.go"}},
		{name: "unrelated owner remains required", reviewers: []string{"@org/api", "@org/core"}, files: []string{"api.go", "other.go"}},
		{name: "signoff does not meet minimum", reviewers: []string{"@org/api", "@org/core"}, files: []string{"api.go"}, minReviews: github.Int(1)},
		{name: "signoff does not reach maximum", reviewers: []string{"@org/api"}, files: []string{"api.go"}, maxReviews: github.Int(1)},
		{name: "ordinary approval plus signoff", reviewers: []string{"@org/api", "@org/core"}, files: []string{"api.go", "other.go"}, ordinaryApprovals: []*gh.CurrentApproval{{Reviewers: codeowners.NewSlugs([]string{"@other"})}}, minReviews: github.Int(1), expectedSuccess: true},
		{name: "ordinary approvals retain enforcement", files: []string{"api.go"}, ordinaryApprovals: []*gh.CurrentApproval{{Reviewers: codeowners.NewSlugs([]string{"@org/api", "@org/core"})}}, expectedSuccess: true, expectedApproval: true},
		{name: "redundant signoff retains enforcement", reviewers: []string{"@org/api", "@org/core"}, files: []string{"api.go"}, ordinaryApprovals: []*gh.CurrentApproval{{Reviewers: codeowners.NewSlugs([]string{"@org/api", "@org/core"})}}, expectedSuccess: true, expectedApproval: true},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			client := &signoffClient{mockGitHubClient: mockGitHubClient{currentApprovals: tc.ordinaryApprovals}}
			if tc.reviewers != nil {
				client.signoffs = []*gh.CurrentApproval{{GHLogin: codeowners.NewSlug("signer"), ReviewID: 10, Reviewers: codeowners.NewSlugs(tc.reviewers), CommitID: "old-head"}}
			}
			app := newSignoffApp(t, client, tc.files)
			app.Conf.MinReviews = tc.minReviews
			app.Conf.MaxReviews = tc.maxReviews
			success, message, _, err := app.processApprovalsAndReviewers()
			if err != nil || success != tc.expectedSuccess {
				t.Fatalf("success = %v, expected %v; %s; error = %v", success, tc.expectedSuccess, message, err)
			}
			if client.approved != tc.expectedApproval {
				t.Errorf("GitHub approval created = %v, expected %v", client.approved, tc.expectedApproval)
			}
			if len(client.dismissed) > 0 {
				t.Errorf("unexpected dismissals: %v", client.dismissed)
			}
		})
	}
}

func TestOwnerSignoffLifetime(t *testing.T) {
	tt := []struct {
		name            string
		disableSmart    bool
		changes         []string
		diffError       error
		expectedSuccess bool
	}{
		{name: "persists across pushes when dismissal disabled", disableSmart: true, changes: []string{"api.go"}, expectedSuccess: true},
		{name: "owned changes invalidate", changes: []string{"api.go"}},
		{name: "unrelated changes preserve", changes: []string{"unrelated.go"}, expectedSuccess: true},
		{name: "unchanged commit preserves", expectedSuccess: true},
		{name: "unreadable commit invalidates", diffError: errors.New("missing commit")},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			client := &signoffClient{signoffs: []*gh.CurrentApproval{{Reviewers: codeowners.NewSlugs([]string{"@org/api", "@org/core"}), CommitID: "reviewed"}}}
			app := newSignoffApp(t, client, []string{"api.go"})
			app.Conf.DisableSmartDismissal = tc.disableSmart
			app.gitDiff = mockGitDiff{changes: tc.changes, changesSinceError: tc.diffError}
			success, message, _, err := app.processApprovalsAndReviewers()
			if err != nil || success != tc.expectedSuccess {
				t.Fatalf("success = %v, expected %v; %s; error = %v", success, tc.expectedSuccess, message, err)
			}
			if client.approved || len(client.dismissed) > 0 {
				t.Errorf("signoff caused GitHub review mutation: approved=%v dismissed=%v", client.approved, client.dismissed)
			}
		})
	}
}

func TestOwnerSignoffReadError(t *testing.T) {
	client := &signoffClient{signoffsError: errors.New("review read failed")}
	app := newSignoffApp(t, client, []string{"api.go"})
	success, _, _, err := app.processApprovalsAndReviewers()
	if err == nil || success {
		t.Fatalf("expected failure, got success=%v error=%v", success, err)
	}
}
