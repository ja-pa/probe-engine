// Package tor contains the tor experiment.
//
// Spec: https://github.com/ooni/spec/blob/master/nettests/ts-023-tor.md
package tor

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ooni/probe-engine/experiment"
	"github.com/ooni/probe-engine/experiment/handler"
	"github.com/ooni/probe-engine/experiment/httpheader"
	"github.com/ooni/probe-engine/internal/netxlogger"
	"github.com/ooni/probe-engine/internal/oonidatamodel"
	"github.com/ooni/probe-engine/internal/oonitemplates"
	testlists "github.com/ooni/probe-engine/internal/orchestra/testlists/tor"
	"github.com/ooni/probe-engine/model"
	"github.com/ooni/probe-engine/session"
)

const (
	testName    = "tor"
	testVersion = "0.0.1"
)

// Config contains the experiment config.
type Config struct{}

// TargetResults contains the results of a target.
type TargetResults struct {
	Agent          string                          `json:"agent"`
	Failure        *string                         `json:"failure"`
	NetworkEvents  oonidatamodel.NetworkEventsList `json:"network_events"`
	Queries        oonidatamodel.DNSQueriesList    `json:"queries"`
	Requests       oonidatamodel.RequestList       `json:"requests"`
	TargetAddress  string                          `json:"target_address"`
	TargetProtocol string                          `json:"target_protocol"`
	TCPConnect     oonidatamodel.TCPConnectList    `json:"tcp_connect"`
	TLSHandshakes  oonidatamodel.TLSHandshakesList `json:"tls_handshakes"`
}

// TestKeys contains tor test keys.
type TestKeys struct {
	Targets map[string]TargetResults `json:"targets"`
}

type measurer struct {
	config Config
}

func newMeasurer(config Config) *measurer {
	return &measurer{config: config}
}

func (m *measurer) measure(
	origCtx context.Context,
	sess *session.Session,
	measurement *model.Measurement,
	callbacks handler.Callbacks,
) error {
	ctx, cancel := context.WithTimeout(origCtx, 60*time.Second)
	defer cancel()
	// fetch experiment targets
	targets, err := testlists.Query(ctx, testlists.Config{})
	if err == nil {
		// measure targets
		err = m.measureTargets(origCtx, sess, measurement, callbacks, targets)
	}
	return err
}

type keytarget struct {
	key    string
	target model.TorTarget
}

func (m *measurer) measureTargets(
	ctx context.Context,
	sess *session.Session,
	measurement *model.Measurement,
	callbacks handler.Callbacks,
	targets map[string]model.TorTarget,
) error {
	// run measurements in parallel
	var waitgroup sync.WaitGroup
	rc := newResultsCollector(sess, measurement, callbacks)
	waitgroup.Add(len(targets))
	workch := make(chan keytarget)
	const parallelism = 2
	for i := 0; i < parallelism; i++ {
		go func(ch <-chan keytarget, total int) {
			for kt := range ch {
				rc.measureSingleTarget(ctx, kt, total)
				waitgroup.Done()
			}
		}(workch, len(targets))
	}
	for key, target := range targets {
		workch <- keytarget{key: key, target: target}
	}
	close(workch)
	waitgroup.Wait()
	// fill the measurement entry
	testkeys := &TestKeys{Targets: rc.targetresults}
	measurement.TestKeys = testkeys
	callbacks.OnDataUsage(
		float64(rc.receivedBytes)/1024.0, // downloaded
		float64(rc.sentBytes)/1024.0,     // uploaded
	)
	return nil
}

type resultsCollector struct {
	callbacks       handler.Callbacks
	completed       int64
	flexibleConnect func(context.Context, model.TorTarget) (oonitemplates.Results, error)
	measurement     *model.Measurement
	mu              sync.Mutex
	receivedBytes   int64
	sentBytes       int64
	sess            *session.Session
	targetresults   map[string]TargetResults
}

func newResultsCollector(
	sess *session.Session,
	measurement *model.Measurement,
	callbacks handler.Callbacks,
) *resultsCollector {
	rc := &resultsCollector{
		callbacks:     callbacks,
		measurement:   measurement,
		sess:          sess,
		targetresults: make(map[string]TargetResults),
	}
	rc.flexibleConnect = rc.defaultFlexibleConnect
	return rc
}

func (rc *resultsCollector) measureSingleTarget(
	ctx context.Context, kt keytarget, total int,
) {
	tk, err := rc.flexibleConnect(ctx, kt.target)
	rc.mu.Lock()
	rc.targetresults[kt.key] = TargetResults{
		Agent:          "redirect",
		Failure:        setFailure(err),
		NetworkEvents:  oonidatamodel.NewNetworkEventsList(tk),
		Queries:        oonidatamodel.NewDNSQueriesList(tk),
		Requests:       oonidatamodel.NewRequestList(tk),
		TargetAddress:  kt.target.Address,
		TargetProtocol: kt.target.Protocol,
		TCPConnect:     oonidatamodel.NewTCPConnectList(tk),
		TLSHandshakes:  oonidatamodel.NewTLSHandshakesList(tk),
	}
	rc.mu.Unlock()
	atomic.AddInt64(&rc.sentBytes, tk.SentBytes)
	atomic.AddInt64(&rc.receivedBytes, tk.ReceivedBytes)
	sofar := atomic.AddInt64(&rc.completed, 1)
	percentage := 0.0
	if total > 0 {
		percentage = float64(sofar) / float64(total)
	}
	rc.callbacks.OnProgress(percentage, fmt.Sprintf(
		"tor: access %s/%s: %s", kt.target.Address, kt.target.Protocol,
		errString(err),
	))
}

func (rc *resultsCollector) defaultFlexibleConnect(
	ctx context.Context, target model.TorTarget,
) (tk oonitemplates.Results, err error) {
	switch target.Protocol {
	case "dir_port":
		url := url.URL{
			Host:   target.Address,
			Path:   "/tor/status-vote/current/consensus.z",
			Scheme: "http",
		}
		const snapshotsize = 1 << 10 // no need to include all in report
		r := oonitemplates.HTTPDo(ctx, oonitemplates.HTTPDoConfig{
			Accept:                  httpheader.RandomAccept(),
			AcceptLanguage:          httpheader.RandomAcceptLanguage(),
			Beginning:               rc.measurement.MeasurementStartTimeSaved,
			MaxEventsBodySnapSize:   snapshotsize,
			MaxResponseBodySnapSize: snapshotsize,
			Handler:                 netxlogger.NewHandler(rc.sess.Logger),
			Method:                  "GET",
			URL:                     url.String(),
			UserAgent:               httpheader.RandomUserAgent(),
		})
		tk, err = r.TestKeys, r.Error
	case "or_port":
		r := oonitemplates.TLSConnect(ctx, oonitemplates.TLSConnectConfig{
			Address:            target.Address,
			Beginning:          rc.measurement.MeasurementStartTimeSaved,
			InsecureSkipVerify: true, // TODO(bassosimone): can we avoid it?
			Handler:            netxlogger.NewHandler(rc.sess.Logger),
		})
		tk, err = r.TestKeys, r.Error
	case "obfs4":
		r := oonitemplates.OBFS4Connect(ctx, oonitemplates.OBFS4ConnectConfig{
			Address:      target.Address,
			Beginning:    rc.measurement.MeasurementStartTimeSaved,
			Handler:      netxlogger.NewHandler(rc.sess.Logger),
			Params:       target.Params,
			StateBaseDir: rc.sess.TempDir,
		})
		tk, err = r.TestKeys, r.Error
	default:
		r := oonitemplates.TCPConnect(ctx, oonitemplates.TCPConnectConfig{
			Address:   target.Address,
			Beginning: rc.measurement.MeasurementStartTimeSaved,
			Handler:   netxlogger.NewHandler(rc.sess.Logger),
		})
		tk, err = r.TestKeys, r.Error
	}
	return
}

// NewExperiment creates a new experiment.
func NewExperiment(
	sess *session.Session, config Config,
) *experiment.Experiment {
	return experiment.New(sess, testName, testVersion,
		newMeasurer(config).measure)
}

func errString(err error) (s string) {
	s = "success"
	if err != nil {
		s = err.Error()
	}
	return
}

func setFailure(err error) (s *string) {
	if err != nil {
		descr := err.Error()
		s = &descr
	}
	return
}
