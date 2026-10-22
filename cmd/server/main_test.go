package main

import "testing"

func TestGradePoints(t *testing.T) {
	tests := map[string]int{"perfect": 4, "good": 3, "almost": 1, "bad": 0, "unknown": 0}
	for grade, want := range tests {
		if got := gradePoints(grade); got != want {
			t.Errorf("gradePoints(%q)=%d, want %d", grade, got, want)
		}
	}
}

func TestValidateAnswerRequiresMeaningfulElements(t *testing.T) {
	grade, feedback := validateAnswer("Нужно применить критерий Коши и проверить условия сходимости последовательности. Затем сделать итоговый вывод.", "критерий Коши + условия + вывод", "Полный ответ")
	if grade != "good" && grade != "perfect" {
		t.Fatalf("grade=%q, feedback=%q", grade, feedback)
	}
	if feedback == "" {
		t.Fatal("validator returned empty feedback")
	}
}

func TestValidateAnswerGivesNextStep(t *testing.T) {
	grade, feedback := validateAnswer("Я применю признак.", "условия + сравнение + вывод", "Полный ответ")
	if grade == "perfect" || feedback == "" {
		t.Fatalf("expected constructive feedback, got grade=%q feedback=%q", grade, feedback)
	}
}
