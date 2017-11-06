package main

import (
	"strings"

	"github.com/pkg/errors"
)

func getAllLabels(labelFile, inputLabels []string) (map[string]string, error) {
	var labelValues []string
	labels := make(map[string]string)
	labelValues, labelErr := readKVStrings(labelFile, inputLabels)
	if labelErr != nil {
		return labels, errors.Wrapf(labelErr, "unable to process labels from --label and label-file")
	}
	// Process KEY=VALUE stringslice in string map for WithLabels func
	if len(labelValues) > 0 {
		for _, i := range labelValues {
			spliti := strings.Split(i, "=")
			if len(spliti) < 2 {
				return labels, errors.Errorf("labels must be in KEY=VALUE format: %s is invalid", i)
			}
			labels[spliti[0]] = spliti[1]
		}
	}
	return labels, nil
}

func convertStringSliceToMap(strSlice []string, delimiter string) (map[string]string, error) {
	sysctl := make(map[string]string)
	for _, inputSysctl := range strSlice {
		values := strings.Split(inputSysctl, delimiter)
		if len(values) < 2 {
			return sysctl, errors.Errorf("%s in an invalid sysctl value", inputSysctl)
		}
		sysctl[values[0]] = values[1]
	}
	return sysctl, nil
}
