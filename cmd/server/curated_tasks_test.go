package main

import (
	"strings"
	"testing"
)

func TestCuratedTaskCatalogCoverage(t *testing.T) {
	seen := make(map[string]struct{}, len(courseTopicOrder))
	for _, title := range courseTopicOrder {
		if _, ok := seen[title]; ok {
			t.Fatalf("duplicate topic in course order: %q", title)
		}
		seen[title] = struct{}{}

		spec, ok := curatedSpecFor(title)
		if !ok {
			t.Fatalf("topic has no curated task specification: %q", title)
		}
		for field, value := range map[string]string{
			"IKR condition": spec.IKR,
			"IKR answer":    spec.IKRAnswer,
			"TKR statement": spec.TKR,
			"TKR answer":    spec.TKRAnswer,
		} {
			if len(strings.TrimSpace(value)) < 40 {
				t.Errorf("%s for %q is too short", field, title)
			}
		}
	}
}

func TestSecondSemesterTopicsAreExplicitlyPlaced(t *testing.T) {
	for _, title := range []string{
		"Теорема об обратной векторной функции",
		"Кратные интегралы",
		"Криволинейные и поверхностные интегралы",
		"Формула Остроградского",
	} {
		if !secondSemesterTopics[title] {
			t.Errorf("expected second-semester topic %q to be explicitly marked", title)
		}
	}
}
