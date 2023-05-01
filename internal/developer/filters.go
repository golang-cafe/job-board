package developer

import (
	"net/url"
	"strconv"
	"strings"
)

type RecruiterFilters = struct {
	HourlyMin  int
	HourlyMax  int
	RoleLevels map[string]interface{}
	RoleTypes  map[string]interface{}
}

func ParseRecruiterFiltersFromQuery(query url.Values) RecruiterFilters {
	hourlyMinStr := query.Get("hourlyMin")
	hourlyMaxStr := query.Get("hourlyMax")
	rawRoleLevelsStr := query.Get("roleLevel")
	rawRoleTypesStr := query.Get("roleType")

	// If we can't convert the string to an int we're happy leaving the zero values
	hourlyMin, _ := strconv.Atoi(hourlyMinStr)
	hourlyMax, _ := strconv.Atoi(hourlyMaxStr)

	// We can take a CSV of role levels.
	roleLevels := make(map[string]interface{})
	for _, rawRoleLevel := range strings.Split(rawRoleLevelsStr, ",") {
		if _, ok := ValidRoleLevels[rawRoleLevel]; ok {
			roleLevels[rawRoleLevel] = true
		}
	}

	// and role types
	roleTypes := make(map[string]interface{})
	for _, rawRoleType := range strings.Split(rawRoleTypesStr, ",") {
		if _, ok := ValidRoleTypes[rawRoleType]; ok {
			roleTypes[rawRoleType] = true
		}
	}

	return RecruiterFilters{
		hourlyMin,
		hourlyMax,
		roleLevels,
		roleTypes,
	}
}
