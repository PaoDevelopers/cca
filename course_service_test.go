package main

import (
	"reflect"
	"testing"
)

func TestPeriodNormalizationPreservesOrderAndComparesAsSet(t *testing.T) {
	input := []string{" Thursday CCA 4 ", "Monday CCA 1", "Thursday CCA 4", ""}
	want := []string{"Thursday CCA 4", "Monday CCA 1"}
	got := normalizeOrderedStringSet(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeOrderedStringSet() = %#v, want %#v", got, want)
	}
	if !equalStringSets(got, []string{"Monday CCA 1", "Thursday CCA 4"}) {
		t.Fatal("equalStringSets() rejected the same periods in canonical SQL order")
	}
	if equalStringSets(got, []string{"Monday CCA 1"}) {
		t.Fatal("equalStringSets() accepted a missing period")
	}
}
