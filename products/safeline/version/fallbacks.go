package version

// fallbackParamRemovals maps command keys to parameters that should be stripped
// as a fallback when a request fails, potentially due to an unsupported parameter.
var fallbackParamRemovals = map[string][]string{
	"log/detect/list": {"exclude_body"},
	"log/access/list": {"exclude_body"},
}

// StripFallbackParams removes known problematic parameters for a given command.
// This is used as a retry mechanism when a request fails and the server version
// is unknown — removing params that old servers may not support.
// The input map is not mutated; a new map is returned.
func StripFallbackParams(commandKey string, params map[string]string) map[string]string {
	removals, ok := fallbackParamRemovals[commandKey]
	if !ok {
		return copyParams(params)
	}

	removalSet := make(map[string]struct{}, len(removals))
	for _, r := range removals {
		removalSet[r] = struct{}{}
	}

	result := make(map[string]string, len(params))
	for k, v := range params {
		if _, shouldRemove := removalSet[k]; shouldRemove {
			continue
		}
		result[k] = v
	}

	return result
}
