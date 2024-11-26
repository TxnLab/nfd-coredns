package nfd_coredns

import (
	"context"
	"reflect"
	"sort"
	"testing"
)

func TestMergeJsonRrrs(t *testing.T) {
	tests := []struct {
		name    string
		base    []JsonRr
		segment []JsonRr
		want    []JsonRr
	}{
		{
			name: "Non-overlapping entries",
			base: []JsonRr{
				{
					Name:    "example.com.",
					Rrdatas: []string{"foo", "bar"},
					Type:    "A",
				},
			},
			segment: []JsonRr{
				{Name: "example.net.", Type: "A"},
			},
			want: []JsonRr{
				{Name: "example.net.", Type: "A"},
				{Name: "example.com.", Type: "A", Rrdatas: []string{"foo", "bar"}},
			},
		},
		{
			name: "Overlapping entries",
			base: []JsonRr{
				{Name: "example.com.", Type: "A", Rrdatas: []string{"base version"}},
			},
			segment: []JsonRr{
				{Name: "example.com.", Type: "A", Rrdatas: []string{"segment version"}},
				{Name: "example.com.", Type: "A", Rrdatas: []string{"segment version 2"}},
			},
			want: []JsonRr{
				{Name: "example.com.", Type: "A", Rrdatas: []string{"base version"}},
			},
		},
		{
			name: "Multiple overlapping entries",
			base: []JsonRr{
				{Name: "example.com.", Type: "A", Rrdatas: []string{"base com version"}},
				{Name: "example.com.", Type: "A", Rrdatas: []string{"base com version 2"}},
				{Name: "example.net.", Type: "A", Rrdatas: []string{"base net version"}},
			},
			segment: []JsonRr{
				{Name: "example.com.", Type: "A", Rrdatas: []string{"segment com version"}},
				{Name: "example.net.", Type: "A", Rrdatas: []string{"segment net version"}},
			},
			want: []JsonRr{
				{Name: "example.com.", Type: "A", Rrdatas: []string{"base com version"}},
				{Name: "example.com.", Type: "A", Rrdatas: []string{"base com version 2"}},
				{Name: "example.net.", Type: "A", Rrdatas: []string{"base net version"}},
			},
		},
		{
			name: "Interleaved overlapping entries",
			base: []JsonRr{
				{Name: "example.com.", Type: "A", Rrdatas: []string{"base com version"}},
				{Name: "example.org.", Type: "A", Rrdatas: []string{"base org version"}},
			},
			segment: []JsonRr{
				{Name: "example.org.", Type: "A", Rrdatas: []string{"segment org version"}},
				{Name: "example.net.", Type: "A", Rrdatas: []string{"segment net version"}},
			},
			want: []JsonRr{
				{Name: "example.com.", Type: "A", Rrdatas: []string{"base com version"}},
				{Name: "example.org.", Type: "A", Rrdatas: []string{"base org version"}},
				{Name: "example.net.", Type: "A", Rrdatas: []string{"segment net version"}},
			},
		},
		{
			name: "Base empty",
			base: []JsonRr{},
			segment: []JsonRr{
				{Name: "example.net.", Type: "A"},
			},
			want: []JsonRr{
				{Name: "example.net.", Type: "A"},
			},
		},
		{
			name: "Segment empty",
			base: []JsonRr{
				{Name: "example.com.", Type: "A"},
			},
			segment: []JsonRr{},
			want: []JsonRr{
				{Name: "example.com.", Type: "A"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeJsonRrrs(context.Background(), tt.base, tt.segment)
			if !equal(got, tt.want) {
				t.Errorf("mergeJsonRrrs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func equal(left []JsonRr, right []JsonRr) bool {
	if len(left) != len(right) {
		return false
	}

	// Sort both slices to make sure the order doesn't affect the comparison
	sort.Slice(left, func(i, j int) bool {
		return left[i].Name < left[j].Name
	})
	sort.Slice(right, func(i, j int) bool {
		return right[i].Name < right[j].Name
	})

	for i := range left {
		if left[i].Name != right[i].Name ||
			!reflect.DeepEqual(left[i].Rrdatas, right[i].Rrdatas) ||
			left[i].Ttl != right[i].Ttl ||
			left[i].Type != right[i].Type {
			return false
		}
	}

	return true
}
