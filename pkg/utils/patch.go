package utils

import (
	"k8s.io/apimachinery/pkg/util/json"
)

// Create a 3-way merge patch based-on JSON merge patch.
// Calculate addition-and-change patch between current and modified.
// Calculate deletion patch between original and modified.
func CreateStatusThreeWayJSONMergePatch(value []byte) ([]byte, error) {
	if len(value) == 0 {
		value = []byte(`{}`)
	}
	var data map[string]interface{} = make(map[string]interface{})
	var in map[string]interface{} = make(map[string]interface{})
	if err := json.Unmarshal(value, &in); err != nil {
		return nil, err
	}
	data["status"] = in
	patch, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return patch, nil
}
