package nasimporter

import (
	"testing"
	"reflect"
)

func setup(t *testing.T) (importer NasImporter) {
	importer, err := NewNasImporter("config.json", false)

	if err != nil {
		t.Error(err)
	}

	return
}

func tearDown() {

}

func TestTVShowRegex1(t *testing.T) {
	importer := setup(t)

	testStringMap := make(map[string]map[string]string)

	testStringMap["show.2009.s01e01.mkv"] = map[string]string{
		"name": "show",
		"year": "2009",
		"season": "01",
		"episode": "01",
		"other": "",
		"ext": "mkv",
	}

	testStringMap["show-_- 2009.S1e1  -dk  -d d d-g .mkv"] = map[string]string{
		"name": "show",
		"year": "2009",
		"season": "1",
		"episode": "1",
		"other": "dk  -d d d-g",
		"ext": "mkv",
	}

	testStringMap["show-_- 2009.S01e1666  -dk  -d d d-g .mkv"] = map[string]string{
		"name": "show",
		"year": "2009",
		"season": "01",
		"episode": "1666",
		"other": "dk  -d d d-g",
		"ext": "mkv",
	}

	testStringMap["show-_- 2009 -_- _..._.- .S01511   e   jkidi fjijfdij f e10  -dk  -d d d-g .mkv"] = map[string]string{
		"name": "show",
		"year": "2009",
		"season": "01511",
		"episode": "10",
		"other": "dk  -d d d-g",
		"ext": "mkv",
	}

	for testString, testGroups := range testStringMap {
		fields := importer.tvShowRegex1.FindStringSubmatchMap(testString)

		if fields == nil {
			t.Errorf("No match for tvShowRegex1:\n%#v\n%#v", importer.tvShowRegex1.String(), testString)
		}

		if !reflect.DeepEqual(fields, testGroups) {
			t.Errorf("Field mismatch:\n%#v\n%#v", testGroups, fields)
		}
	}
}

//start putting year in tvdb search
func TestTVShowRegex2(t *testing.T) {
	importer := setup(t)

	testStringMap := make(map[string]map[string]string)

	testStringMap["show.s01e01.mkv"] = map[string]string{
		"name": "show",
		"season": "01",
		"episode": "01",
		"other": "",
		"ext": "mkv",
	}

	testStringMap["show-_- .S1e1  -dk  -d d d-g .mkv"] = map[string]string{
		"name": "show",
		"season": "1",
		"episode": "1",
		"other": "dk  -d d d-g",
		"ext": "mkv",
	}

	testStringMap["show-_- .S01e1666  -dk  -d d d-g .mkv"] = map[string]string{
		"name": "show",
		"season": "01",
		"episode": "1666",
		"other": "dk  -d d d-g",
		"ext": "mkv",
	}

	testStringMap["show-_-  -_- _..._.- .S01511   e   jkidi fjijfdij f e10  -dk  -d d d-g .mkv"] = map[string]string{
		"name": "show",
		"season": "01511",
		"episode": "10",
		"other": "dk  -d d d-g",
		"ext": "mkv",
	}

	for testString, testGroups := range testStringMap {
		fields := importer.tvShowRegex2.FindStringSubmatchMap(testString)

		if fields == nil {
			t.Errorf("No match for tvShowRegex2:\n%#v\n%#v", importer.tvShowRegex2.String(), testString)
		}

		if !reflect.DeepEqual(fields, testGroups) {
			t.Errorf("Field mismatch:\n%#v\n%#v", testGroups, fields)
		}
	}
}
