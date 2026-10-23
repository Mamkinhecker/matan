package main

import "testing"

func TestCuratedQuestionCatalogHas336IKRAndTKRTasks(t *testing.T) {
	if len(curatedFifthTaskSpecs) != 69 {
		t.Fatalf("new authored topic pairs = %d, want 69", len(curatedFifthTaskSpecs))
	}
	counts := map[string]int{"ikr": len(courseTopicOrder), "tkr": len(courseTopicOrder)}
	for _, task := range curatedExtraTasks {
		counts[task.kind]++
	}
	for _, task := range curatedSecondTasks() {
		counts[task.kind]++
	}
	if counts["ikr"] != 336 || counts["tkr"] != 336 {
		t.Fatalf("curated catalog counts are IKR/TKR %d/%d, want 336/336", counts["ikr"], counts["tkr"])
	}
}

func TestCuratedSecondTaskCatalog(t *testing.T) {
	if err := validateCuratedSecondTaskCatalog(); err != nil {
		t.Fatal(err)
	}
	for _, task := range curatedSecondTasks() {
		if grade, _ := validateAnswer(task.answer, task.short, task.answer); grade != "perfect" {
			t.Errorf("exact authored solution for %q (%s) was graded %q", task.topic, task.kind, grade)
		}
	}
}
