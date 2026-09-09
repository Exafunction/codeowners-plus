package gh

import (
	"strings"

	"github.com/google/go-github/v63/github"
	"github.com/multimediallc/codeowners-plus/pkg/directives"
)

func (gh *GHClient) GetCurrentOwnerSignoffs() ([]*CurrentApproval, error) {
	if gh.pr == nil {
		return nil, &NoPRError{}
	}
	if gh.userReviewerMap == nil {
		return nil, &UserReviewerMapNotInitError{}
	}
	if gh.reviews == nil {
		if err := gh.InitReviews(); err != nil {
			return nil, err
		}
	}
	seen := make(map[string]bool)
	signoffs := make([]*github.PullRequestReview, 0)
	for _, review := range gh.reviews {
		user := strings.ToLower(review.GetUser().GetLogin())
		if seen[user] {
			continue
		}
		switch review.GetState() {
		case "CHANGES_REQUESTED", "DISMISSED":
			seen[user] = true
		case "COMMENTED":
			if directives.HasCodeownersApproval(review.GetBody()) {
				seen[user] = true
				signoffs = append(signoffs, review)
			}
		}
	}
	return currentReviewerApprovalsFromReviews(signoffs, gh.userReviewerMap), nil
}
