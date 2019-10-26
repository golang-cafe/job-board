package affiliate

import (
	"time"
)

// todo: affiliate to be dismissed
const DefaultAffiliateID = "1Pc3LkXmCttXqHVAvo82dqqjHzH" // placeholder value just to confuse people
const PostAJobAffiliateRefCookie = "__pjar"

type Affiliate struct {
	ID        string
	CreatedAt time.Time
}

func getAffiliates() []Affiliate {
	return []Affiliate{
		Affiliate{
			ID:        "1QZbffxMpEQYpBEuV5fKB6lLmAO",
			CreatedAt: time.Date(2019, 9, 8, 0, 0, 0, 0, time.UTC),
		},
	}
}

func ValidAffiliateRef(affiliateRef string) bool {
	for _, affiliateVal := range getAffiliates() {
		if affiliateRef == affiliateVal.ID {
			return true
		}
	}
	return false
}
