package gh

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/google/go-github/v63/github"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

func ownerReview(login, state, body string, id int64) *github.PullRequestReview {
	return &github.PullRequestReview{
		User:     &github.User{Login: github.String(login)},
		State:    github.String(state),
		Body:     github.String(body),
		ID:       github.Int64(id),
		CommitID: github.String("reviewed-commit"),
	}
}

func TestCurrentOwnerSignoffs(t *testing.T) {
	signoff := ownerReview("OwNeR", "COMMENTED", "codeowners-approved", 1)
	tt := []struct {
		name     string
		reviews  []*github.PullRequestReview
		expected []int64
	}{
		{"signoff", []*github.PullRequestReview{signoff}, []int64{1}},
		{"unrelated comment preserves", []*github.PullRequestReview{ownerReview("owner", "COMMENTED", "Thanks", 2), signoff}, []int64{1}},
		{"request changes revokes", []*github.PullRequestReview{ownerReview("owner", "CHANGES_REQUESTED", "", 2), signoff}, nil},
		{"dismissal revokes", []*github.PullRequestReview{ownerReview("owner", "DISMISSED", "codeowners-approved", 2), signoff}, nil},
		{"new signoff restores", []*github.PullRequestReview{signoff, ownerReview("owner", "CHANGES_REQUESTED", "", 2)}, []int64{1}},
		{"newest signoff per user", []*github.PullRequestReview{ownerReview("owner", "COMMENTED", "codeowners-approved", 2), signoff}, []int64{2}},
		{"ordinary approval is separate", []*github.PullRequestReview{ownerReview("owner", "APPROVED", "codeowners-approved", 2)}, nil},
		{"pending review is ignored", []*github.PullRequestReview{ownerReview("owner", "PENDING", "codeowners-approved", 2)}, nil},
		{"invalid directive", []*github.PullRequestReview{ownerReview("owner", "COMMENTED", "```\ncodeowners-approved\n```", 2)}, nil},
		{"different reviewer cannot revoke", []*github.PullRequestReview{ownerReview("other", "CHANGES_REQUESTED", "", 2), signoff}, []int64{1}},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			client := &GHClient{
				pr:              &github.PullRequest{Number: github.Int(1)},
				reviews:         tc.reviews,
				userReviewerMap: ghUserReviewerMap{"owner": codeowners.NewSlugs([]string{"@org/api", "@org/core"})},
			}
			signoffs, err := client.GetCurrentOwnerSignoffs()
			if err != nil {
				t.Fatal(err)
			}
			var ids []int64
			for _, signoff := range signoffs {
				ids = append(ids, signoff.ReviewID)
				expected := []string{"@org/api", "@org/core"}
				if !reflect.DeepEqual(codeowners.OriginalStrings(signoff.Reviewers), expected) {
					t.Errorf("reviewers = %v, expected %v", signoff.Reviewers, expected)
				}
				if signoff.CommitID != "reviewed-commit" {
					t.Errorf("commit = %q", signoff.CommitID)
				}
			}
			if !reflect.DeepEqual(ids, tc.expected) {
				t.Errorf("signoff IDs = %v, expected %v", ids, tc.expected)
			}
		})
	}
}

func TestOwnerSignoffEditAndApprovalSeparation(t *testing.T) {
	review := ownerReview("reviewer1", "COMMENTED", "codeowners-approved", 10)
	client := setupReviews()
	client.reviews = []*github.PullRequestReview{review}
	signoffs, err := client.GetCurrentOwnerSignoffs()
	if err != nil || len(signoffs) != 1 {
		t.Fatalf("signoffs = %v, error = %v", signoffs, err)
	}
	approvals, err := client.GetCurrentReviewerApprovals()
	if err != nil || len(approvals) != 0 {
		t.Fatalf("signoff counted as ordinary approval: %v, %v", approvals, err)
	}
	allApprovals, err := client.AllApprovals()
	if err != nil || len(allApprovals) != 0 {
		t.Fatalf("signoff counted in AllApprovals: %v, %v", allApprovals, err)
	}
	approval, err := client.FindUserApproval("reviewer1")
	if err != nil || approval != nil {
		t.Fatalf("signoff counted by FindUserApproval: %v, %v", approval, err)
	}
	review.Body = github.String("Withdrawn")
	signoffs, err = client.GetCurrentOwnerSignoffs()
	if err != nil || len(signoffs) != 0 {
		t.Fatalf("removed signoff still counts: %v, %v", signoffs, err)
	}
}

func TestOwnerSignoffMembershipAndAuthorExclusion(t *testing.T) {
	reviews := []*github.PullRequestReview{
		ownerReview("author", "COMMENTED", "codeowners-approved", 1),
		ownerReview("Teammate", "COMMENTED", "codeowners-approved", 2),
		ownerReview("outsider", "COMMENTED", "codeowners-approved", 3),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/org/repo/pulls/1/reviews" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(reviews); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	api := github.NewClient(server.Client())
	api.BaseURL, _ = url.Parse(server.URL + "/")
	client := &GHClient{
		ctx: context.Background(), owner: "org", repo: "repo", client: api,
		pr: &github.PullRequest{Number: github.Int(1), User: &github.User{Login: github.String("author")}},
		userReviewerMap: makeGHUserReviwerMap([]string{"@org/api", "@org/core", "@teammate"}, func(org, team string) []*github.User {
			return []*github.User{{Login: github.String("teammate")}}
		}),
		warningBuffer: io.Discard, infoBuffer: io.Discard,
	}
	signoffs, err := client.GetCurrentOwnerSignoffs()
	if err != nil {
		t.Fatal(err)
	}
	if len(signoffs) != 2 {
		t.Fatalf("signoffs = %v, expected author excluded", signoffs)
	}
	if len(signoffs[0].Reviewers) != 0 {
		t.Errorf("outsider satisfies owners: %v", signoffs[0].Reviewers)
	}
	expected := []string{"@org/api", "@org/core", "@teammate"}
	if !reflect.DeepEqual(codeowners.OriginalStrings(signoffs[1].Reviewers), expected) {
		t.Errorf("team/user mapping = %v, expected %v", signoffs[1].Reviewers, expected)
	}
}

func TestOwnerSignoffInitializationErrors(t *testing.T) {
	client := &GHClient{}
	if _, err := client.GetCurrentOwnerSignoffs(); !errors.As(err, new(*NoPRError)) {
		t.Fatalf("expected NoPRError, got %v", err)
	}
	client.pr = &github.PullRequest{}
	if _, err := client.GetCurrentOwnerSignoffs(); !errors.As(err, new(*UserReviewerMapNotInitError)) {
		t.Fatalf("expected UserReviewerMapNotInitError, got %v", err)
	}
}
