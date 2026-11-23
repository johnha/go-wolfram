package wolfram

import (
	"testing"
)

func TestGeneralizationObjectOrList(t *testing.T) {
	var q Query
	obj := []byte(`{"queryresult":{"success":true,"error":false,"numpods":0,"generalization":{"topic":"NVIDIA","desc":"General results for:","id":"x"}}}`)
	if err := jsonLib.Unmarshal(obj, &q); err != nil {
		t.Fatalf("object: %v", err)
	}
	if len(q.Result.Generalizations) != 1 || q.Result.Generalizations[0].Topic != "NVIDIA" {
		t.Fatalf("got %+v", q.Result.Generalizations)
	}

	arr := []byte(`{"queryresult":{"success":true,"error":false,"numpods":0,"generalization":[{"topic":"A"},{"topic":"B"}]}}`)
	q = Query{}
	if err := jsonLib.Unmarshal(arr, &q); err != nil {
		t.Fatalf("array: %v", err)
	}
	if len(q.Result.Generalizations) != 2 {
		t.Fatalf("got %d", len(q.Result.Generalizations))
	}
}
