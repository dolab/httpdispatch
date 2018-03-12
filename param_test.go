package httpdispatch

import "testing"

func TestParams(t *testing.T) {
	ps := Params{
		Param{"param1", "value1"},
		Param{"param2", "value2"},
		Param{"param3", "value3"},
	}
	for i := range ps {
		if val := ps.ByName(ps[i].Key); val != ps[i].Value {
			t.Errorf("Wrong value for %s: Got %s; Want %s", ps[i].Key, val, ps[i].Value)
		}

		if _, ok := ps.DefName(ps[i].Key); !ok {
			t.Errorf("Wrong value for %s: Got %v; Want true", ps[i].Key, ok)
		}
	}

	if val := ps.ByName("noKey"); val != "" {
		t.Errorf("Expected empty string for not found key; got: %s", val)
	}

	if _, ok := ps.DefName("noKey"); ok {
		t.Errorf("Expected false for not found key; got: %v", ok)
	}
}
