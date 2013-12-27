package mapregexp

import (
	"regexp"
)

type MapRegexp struct {
	regexp.Regexp
}

func MustCompile(pattern string) *MapRegexp {
	return &MapRegexp{*regexp.MustCompile(pattern)}
}

func (regex *MapRegexp) FindStringSubmatchMap(str string) map[string]string {
	submatchMap := make(map[string]string)
	submatches := regex.FindStringSubmatch(str)

	if submatches == nil {
		return nil
	}

	for index, name := range regex.SubexpNames() {
		if index == 0 || name == "" {
			continue
		}

		submatchMap[name] = submatches[index]
	}

	return submatchMap
}
