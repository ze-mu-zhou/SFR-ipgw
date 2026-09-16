package utils

import (
	"errors"
	"regexp"
)

var errorNoMatched = errors.New("no matched")

func MatchMultiple(re *regexp.Regexp, content string) ([][]string, error) {
	matched := re.FindAllStringSubmatch(content, -1)
	if len(matched) < 1 {
		return nil, errorNoMatched
	}
	return matched, nil
}

func MatchSingle(re *regexp.Regexp, content string) (string, error) {
	matched := re.FindAllStringSubmatch(content, -1)
	if len(matched) < 1 {
		return "", errorNoMatched
	}
	return matched[0][1], nil
}
