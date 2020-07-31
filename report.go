package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

type Report struct {
	w *csv.Writer
}

var header = []string {"Project", "Type", "Resource Name", "Tags", "Missing Tags", "Created By", "Classic Coverage",
	"Modern Coverage",}
var classic = []string {"Name","BU","Product","Repository","TeamID","Environment"}
var modern = []string {"Name","rlg:business-unit","rlg:product","rlg:application","rlg:repository","rlg:techdata-team",
	"rlg:contact","rlg:environment","rlg:classification","rlg:compliance"}

func NewReporter() *Report {
	var report = &Report{
		w: csv.NewWriter(os.Stdout),
	}
	err := report.w.Write(header)
	if err != nil {
		panic(err)
	}
	return report
}

func (r Report) Add(resourceType string, name string, stack string, search string, tags map[string]string) {
	hasModern, missModern := extractKeys(tags, modern)
	hasClassic, _ := extractKeys(tags, classic)

	err := r.w.Write([]string {
		search,
		extractType(resourceType),
		name,
		strings.Join(hasModern, ","),
		strings.Join(missModern, ","),
		extractOrigin(stack, search),
		fmt.Sprintf("%d%%", 100*len(hasClassic)/len(classic)),
		fmt.Sprintf("%d%%", 100*len(hasModern)/len(modern)),
	})

	if err != nil {
		panic(err.Error())
	}
}

func (r Report) NotSupported(resourceType string, name string, stack string, search string) {
	err := r.w.Write([]string {
		search,
		extractType(resourceType),
		name,
		"",
		"",
		extractOrigin(stack, search),
		"N/A",
		"N/A",
	})

	if err != nil {
		panic(err.Error())
	}
}

func (r Report) Write() {
	r.w.Flush()
	err := r.w.Error()
	if err != nil {
		panic(err.Error())
	}
}

func extractType(resourceType string) string {
	split := strings.Split(resourceType, "::")
	if len(split) > 2 {
		return split[2]
	} else {
		return resourceType
	}
}

func extractOrigin(stack string, search string) string {
	if strings.Contains(stack, search) {
		return "PIPELINE"
	} else {
		return "CUSTOM"
	}
}

func extractKeys(sample map[string]string, required []string) ([]string, []string) {
	var has []string
	var miss []string
	for _, key := range required {
		if _, ok := sample[key]; ok {
			has = append(has, key)
		} else {
			miss = append(miss, key)
		}
	}
	return has, miss
}
