package main

import "testing"

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
