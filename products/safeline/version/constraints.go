package version

// ParamConstraint defines the minimum server version required for a parameter.
type ParamConstraint struct {
	MinVersion string // minimum server version supporting this param
}

// paramConstraints maps command keys to their parameter constraints.
// Parameters listed here are only sent to the server if the server version
// meets or exceeds the MinVersion.
var paramConstraints = map[string]map[string]ParamConstraint{
	"log/detect/list": {
		"exclude_body": {MinVersion: "23.02.001"},
	},
	"log/access/list": {
		"exclude_body": {MinVersion: "23.02.001"},
	},
}

// FilterParams removes parameters that the server version does not support.
// If serverVersion is empty or cannot be parsed, all params are returned unchanged.
// The input map is not mutated; a new map is returned.
func FilterParams(commandKey string, params map[string]string, serverVersion string) map[string]string {
	if serverVersion == "" {
		return copyParams(params)
	}

	serverVer, err := ParseVersion(serverVersion)
	if err != nil {
		return copyParams(params)
	}

	constraints, ok := paramConstraints[commandKey]
	if !ok {
		return copyParams(params)
	}

	result := make(map[string]string, len(params))
	for k, v := range params {
		constraint, hasConstraint := constraints[k]
		if !hasConstraint {
			result[k] = v
			continue
		}

		minVer, err := ParseVersion(constraint.MinVersion)
		if err != nil {
			// If the constraint version is invalid, keep the param
			result[k] = v
			continue
		}

		if !serverVer.LessThan(minVer) {
			result[k] = v
		}
		// If server version is less than min, skip this param
	}

	return result
}

// copyParams returns a shallow copy of the params map.
func copyParams(params map[string]string) map[string]string {
	result := make(map[string]string, len(params))
	for k, v := range params {
		result[k] = v
	}
	return result
}
