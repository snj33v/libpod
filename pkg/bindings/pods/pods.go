package pods

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/pkg/bindings"
	"github.com/containers/libpod/pkg/domain/entities"
	"github.com/containers/libpod/pkg/specgen"
	jsoniter "github.com/json-iterator/go"
)

func CreatePodFromSpec(ctx context.Context, s *specgen.PodSpecGenerator) (*entities.PodCreateReport, error) {
	var (
		pcr entities.PodCreateReport
	)
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	specgenString, err := jsoniter.MarshalToString(s)
	if err != nil {
		return nil, err
	}
	stringReader := strings.NewReader(specgenString)
	response, err := conn.DoRequest(stringReader, http.MethodPost, "/pods/create", nil)
	if err != nil {
		return nil, err
	}
	return &pcr, response.Process(&pcr)
}

// Exists is a lightweight method to determine if a pod exists in local storage
func Exists(ctx context.Context, nameOrID string) (bool, error) {
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return false, err
	}
	response, err := conn.DoRequest(nil, http.MethodGet, "/pods/%s/exists", nil, nameOrID)
	if err != nil {
		return false, err
	}
	return response.IsSuccess(), nil
}

// Inspect returns low-level information about the given pod.
func Inspect(ctx context.Context, nameOrID string) (*libpod.PodInspect, error) {
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	inspect := libpod.PodInspect{}
	response, err := conn.DoRequest(nil, http.MethodGet, "/pods/%s/json", nil, nameOrID)
	if err != nil {
		return &inspect, err
	}
	return &inspect, response.Process(&inspect)
}

// Kill sends a SIGTERM to all the containers in a pod.  The optional signal parameter
// can be used to override  SIGTERM.
func Kill(ctx context.Context, nameOrID string, signal *string) (*entities.PodKillReport, error) {
	var (
		report entities.PodKillReport
	)
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	if signal != nil {
		params.Set("signal", *signal)
	}
	response, err := conn.DoRequest(nil, http.MethodPost, "/pods/%s/kill", params, nameOrID)
	if err != nil {
		return nil, err
	}
	return &report, response.Process(&report)
}

// Pause pauses all running containers in a given pod.
func Pause(ctx context.Context, nameOrID string) (*entities.PodPauseReport, error) {
	var report entities.PodPauseReport
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	response, err := conn.DoRequest(nil, http.MethodPost, "/pods/%s/pause", nil, nameOrID)
	if err != nil {
		return nil, err
	}
	return &report, response.Process(&report)
}

// Prune removes all non-running pods in local storage.
func Prune(ctx context.Context) error {
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return err
	}
	response, err := conn.DoRequest(nil, http.MethodPost, "/pods/prune", nil)
	if err != nil {
		return err
	}
	return response.Process(nil)
}

// List returns all pods in local storage.  The optional filters parameter can
// be used to refine which pods should be listed.
func List(ctx context.Context, filters map[string][]string) ([]*entities.ListPodsReport, error) {
	var (
		podsReports []*entities.ListPodsReport
	)
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	if filters != nil {
		stringFilter, err := bindings.FiltersToString(filters)
		if err != nil {
			return nil, err
		}
		params.Set("filters", stringFilter)
	}
	response, err := conn.DoRequest(nil, http.MethodGet, "/pods/json", params)
	if err != nil {
		return podsReports, err
	}
	return podsReports, response.Process(&podsReports)
}

// Restart restarts all containers in a pod.
func Restart(ctx context.Context, nameOrID string) (*entities.PodRestartReport, error) {
	var report entities.PodRestartReport
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	response, err := conn.DoRequest(nil, http.MethodPost, "/pods/%s/restart", nil, nameOrID)
	if err != nil {
		return nil, err
	}
	return &report, response.Process(&report)
}

// Remove deletes a Pod from from local storage. The optional force parameter denotes
// that the Pod can be removed even if in a running state.
func Remove(ctx context.Context, nameOrID string, force *bool) (*entities.PodRmReport, error) {
	var report entities.PodRmReport
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	if force != nil {
		params.Set("force", strconv.FormatBool(*force))
	}
	response, err := conn.DoRequest(nil, http.MethodDelete, "/pods/%s", params, nameOrID)
	if err != nil {
		return nil, err
	}
	return &report, response.Process(&report)
}

// Start starts all containers in a pod.
func Start(ctx context.Context, nameOrID string) (*entities.PodStartReport, error) {
	var report entities.PodStartReport
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	response, err := conn.DoRequest(nil, http.MethodPost, "/pods/%s/start", nil, nameOrID)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotModified {
		report.Id = nameOrID
		return &report, nil
	}
	return &report, response.Process(&report)
}

func Stats() error {
	// TODO
	return bindings.ErrNotImplemented
}

// Stop stops all containers in a Pod. The optional timeout parameter can be
// used to override the timeout before the container is killed.
func Stop(ctx context.Context, nameOrID string, timeout *int) (*entities.PodStopReport, error) {
	var report entities.PodStopReport
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	if timeout != nil {
		params.Set("t", strconv.Itoa(*timeout))
	}
	response, err := conn.DoRequest(nil, http.MethodPost, "/pods/%s/stop", params, nameOrID)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotModified {
		report.Id = nameOrID
		return &report, nil
	}
	return &report, response.Process(&report)
}

func Top() error {
	// TODO
	return bindings.ErrNotImplemented // nolint:typecheck
}

// Unpause unpauses all paused containers in a Pod.
func Unpause(ctx context.Context, nameOrID string) (*entities.PodUnpauseReport, error) {
	var report entities.PodUnpauseReport
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	response, err := conn.DoRequest(nil, http.MethodPost, "/pods/%s/unpause", nil, nameOrID)
	if err != nil {
		return nil, err
	}
	return &report, response.Process(&report)
}
