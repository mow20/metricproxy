package forwarder

import (
	"github.com/cep21/gohelpers/a"
	"github.com/cep21/gohelpers/workarounds"
	"github.com/signalfuse/com_signalfuse_metrics_protobuf"
	"github.com/signalfuse/signalfxproxy/config"
	"github.com/signalfuse/signalfxproxy/core"
	"github.com/signalfuse/signalfxproxy/core/value"
	"github.com/signalfuse/signalfxproxy/listener"
	"testing"
	//	"time"
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"time"
)

func TestBodySendFormat(t *testing.T) {
	b := &BodySendFormat{Metric: "cpu"}
	a.ExpectContains(t, b.String(), "cpu", "Expect cpu")
}

type metricPanicDatapoint struct {
	core.Datapoint
}

func (vp *metricPanicDatapoint) Metric() string {
	panic("This shouldn't happen!")
}

func TestSignalfxJSONForwarderLoader(t *testing.T) {
	listenFrom := config.ListenFrom{}
	listenFrom.ListenAddr = workarounds.GolangDoesnotAllowPointerToStringLiteral("0.0.0.0:12345")

	forwardTo := config.ForwardTo{
		URL:               workarounds.GolangDoesnotAllowPointerToStringLiteral("http://0.0.0.0:12345/v1/datapoint"),
		TimeoutDuration:   workarounds.GolangDoesnotAllowPointerToDurationLiteral(time.Second * 1),
		MetricCreationURL: workarounds.GolangDoesnotAllowPointerToStringLiteral("http://0.0.0.0:12345/v1/metric"),
		DefaultAuthToken:  workarounds.GolangDoesnotAllowPointerToStringLiteral("AUTH_TOKEN"),
		DefaultSource:     workarounds.GolangDoesnotAllowPointerToStringLiteral("proxy-source"),
	}

	finalDatapointDestination := newBasicBufferedForwarder(100, 1, "", 1)
	l, err := listener.SignalFxListenerLoader(finalDatapointDestination, &listenFrom)
	defer l.Close()
	a.ExpectEquals(t, nil, err, "Expect no error")

	forwarder, err := SignalfxJSONForwarderLoader(&forwardTo)
	a.ExpectEquals(t, nil, err, "Expect no error")
	a.ExpectEquals(t, "signalfx-forwarder", forwarder.Name(), "Expect no error")
	a.ExpectEquals(t, 0, len(forwarder.GetStats()), "Expect no stats")

	sfForwarder, _ := forwarder.(*signalfxJSONConnector)

	timeToSend := time.Now().Round(time.Second)
	dpSent := core.NewAbsoluteTimeDatapoint("metric", map[string]string{}, value.NewIntWire(2), com_signalfuse_metrics_protobuf.MetricType_GAUGE, timeToSend)
	forwarder.DatapointsChannel() <- dpSent
	dpRecieved := <-finalDatapointDestination.datapointsChannel
	i, _ := dpRecieved.Value().IntValue()
	a.ExpectEquals(t, int64(2), i, "Expect 2 back")

	dpSent = core.NewAbsoluteTimeDatapoint("metric", map[string]string{}, value.NewFloatWire(2), com_signalfuse_metrics_protobuf.MetricType_GAUGE, timeToSend)
	forwarder.DatapointsChannel() <- dpSent
	dpRecieved = <-finalDatapointDestination.datapointsChannel
	f, _ := dpRecieved.Value().FloatValue()
	a.ExpectEquals(t, 2.0, f, "Expect 2 back")

	dpStr := core.NewAbsoluteTimeDatapoint("metric", map[string]string{}, value.NewStrWire("astr"), com_signalfuse_metrics_protobuf.MetricType_GAUGE, timeToSend)
	forwarder.DatapointsChannel() <- dpStr
	dpRecieved = <-finalDatapointDestination.datapointsChannel
	a.ExpectEquals(t, "astr", dpRecieved.Value().WireValue(), "Expect 2 back")

	// No source should mean we don't ask for the metric
	sfForwarder.defaultSource = ""
	dp := &metricPanicDatapoint{dpSent}
	err = sfForwarder.process([]core.Datapoint{dp})
	sfForwarder.defaultSource = "proxy"
	a.ExpectEquals(t, nil, err, "Expect no error")

	dpSent = core.NewAbsoluteTimeDatapoint("metric", map[string]string{}, value.NewFloatWire(2.0), com_signalfuse_metrics_protobuf.MetricType_COUNTER, timeToSend)
	forwarder.DatapointsChannel() <- dpSent
	dpRecieved = <-finalDatapointDestination.datapointsChannel
	f, _ = dpRecieved.Value().FloatValue()
	a.ExpectEquals(t, 2.0, f, "Expect 2 back")
	a.ExpectEquals(t, com_signalfuse_metrics_protobuf.MetricType_COUNTER, dpRecieved.MetricType(), "Expect 2 back")

	dpSent = core.NewRelativeTimeDatapoint("metric", map[string]string{}, value.NewFloatWire(2.0), com_signalfuse_metrics_protobuf.MetricType_COUNTER, -1)
	forwarder.DatapointsChannel() <- dpSent
	dpRecieved = <-finalDatapointDestination.datapointsChannel
	ts, _ := dpRecieved.(core.TimeRelativeDatapoint)
	a.ExpectEquals(t, int64(-1), ts.RelativeTime(), "Expect -1 time ago")

	dpSent = core.NewRelativeTimeDatapoint("metricnowacounter", map[string]string{"sf_source": "asource"}, value.NewFloatWire(2.0), com_signalfuse_metrics_protobuf.MetricType_COUNTER, -1)
	forwarder.DatapointsChannel() <- dpSent
	dpRecieved = <-finalDatapointDestination.datapointsChannel
	_, ok := sfForwarder.v1MetricLoadedCache["metricnowacounter"]
	a.ExpectEquals(t, true, ok, "Expected asource")
	a.ExpectEquals(t, "asource", dpRecieved.Dimensions()["sf_source"], "Expected asource")

	sfForwarder.MetricCreationURL = "http://0.0.0.0:21/asfd" // invalid
	dpSent = core.NewRelativeTimeDatapoint("anotermetric", map[string]string{}, value.NewFloatWire(2.0), com_signalfuse_metrics_protobuf.MetricType_COUNTER, -1)
	sfForwarder.process([]core.Datapoint{dpSent})
	a.ExpectEquals(t, 0, len(finalDatapointDestination.datapointsChannel), "Expect no metrics")
	sfForwarder.MetricCreationURL = "http://0.0.0.0:12345/v1/metric"

	err = sfForwarder.createMetricsOfType(map[string]com_signalfuse_metrics_protobuf.MetricType{})
	a.ExpectEquals(t, nil, err, "Expected no error making no metrics")

	jsonXXXMarshal = func(interface{}) ([]byte, error) { return nil, errors.New("json marshal issue") }
	err = sfForwarder.createMetricsOfType(map[string]com_signalfuse_metrics_protobuf.MetricType{"m": com_signalfuse_metrics_protobuf.MetricType_COUNTER})
	a.ExpectNotEquals(t, nil, err, "Expected no error making no metrics")
	jsonXXXMarshal = json.Marshal

	ioutilXXXReadAll = func(r io.Reader) ([]byte, error) { return nil, errors.New("ioutil") }
	err = sfForwarder.createMetricsOfType(map[string]com_signalfuse_metrics_protobuf.MetricType{"m": com_signalfuse_metrics_protobuf.MetricType_COUNTER})
	a.ExpectContains(t, err.Error(), "ioutil", "Expected ioutil issue")
	ioutilXXXReadAll = ioutil.ReadAll

	sfForwarder.MetricCreationURL = "http://0.0.0.0:12345/vmetric"
	err = sfForwarder.createMetricsOfType(map[string]com_signalfuse_metrics_protobuf.MetricType{"m": com_signalfuse_metrics_protobuf.MetricType_COUNTER})
	a.ExpectContains(t, err.Error(), "invalid status code", "Expected status code 404")
	sfForwarder.MetricCreationURL = "http://0.0.0.0:12345/v1/metric"

	ioutilXXXReadAll = func(r io.Reader) ([]byte, error) { return []byte("InvalidJson"), nil }
	err = sfForwarder.createMetricsOfType(map[string]com_signalfuse_metrics_protobuf.MetricType{"m": com_signalfuse_metrics_protobuf.MetricType_COUNTER})
	a.ExpectContains(t, err.Error(), "invalid character", "Expected ioutil issue")
	ioutilXXXReadAll = ioutil.ReadAll

	ioutilXXXReadAll = func(r io.Reader) ([]byte, error) { return []byte("InvalidJson"), nil }
	err = sfForwarder.createMetricsOfType(map[string]com_signalfuse_metrics_protobuf.MetricType{"m": com_signalfuse_metrics_protobuf.MetricType_COUNTER})
	a.ExpectContains(t, err.Error(), "invalid character", "Expected ioutil issue")
	ioutilXXXReadAll = ioutil.ReadAll

	ioutilXXXReadAll = func(r io.Reader) ([]byte, error) { return []byte(`[{"code":203}]`), nil }
	err = sfForwarder.createMetricsOfType(map[string]com_signalfuse_metrics_protobuf.MetricType{"wontexist": com_signalfuse_metrics_protobuf.MetricType_COUNTER})
	a.ExpectEquals(t, nil, err, "Expected no error making no metrics")
	_, ok = sfForwarder.v1MetricLoadedCache["wontexist"]
	a.ExpectEquals(t, false, ok, "Should not make")
	ioutilXXXReadAll = ioutil.ReadAll

	protoXXXMarshal = func(r proto.Message) ([]byte, error) { return nil, errors.New("proto encode error") }
	_, _, err = sfForwarder.encodePostBodyV1([]core.Datapoint{dpSent})
	a.ExpectEquals(t, "proto encode error", err.Error(), "Expected error encoding protobufs")
	protoXXXMarshal = proto.Marshal

	sfForwarder.sendVersion = 2
	_, _, err = sfForwarder.encodePostBody([]core.Datapoint{dpSent, dpStr})
	sfForwarder.sendVersion = 1
	a.ExpectEquals(t, nil, err, "Expected no error making no metrics")
	protoXXXMarshal = proto.Marshal

	prevURL := sfForwarder.url
	sfForwarder.url = "http://0.0.0.0:12333/vvv/s"
	err = sfForwarder.process([]core.Datapoint{dpSent})
	a.ExpectContains(t, err.Error(), "connection refused", "Expected error posting points")
	sfForwarder.url = prevURL

	sfForwarder.url = "http://0.0.0.0:12345/v1/metric"
	err = sfForwarder.process([]core.Datapoint{dpSent})
	a.ExpectContains(t, err.Error(), "invalid status code", "Expected error posting points to metric creation url")
	sfForwarder.url = prevURL

	ioutilXXXReadAll = func(r io.Reader) ([]byte, error) { return nil, errors.New("ioutil") }
	err = sfForwarder.process([]core.Datapoint{dpSent})
	a.ExpectEquals(t, "ioutil", err.Error(), "Expected ioutil decoding response")
	ioutilXXXReadAll = ioutil.ReadAll

	jsonXXXUnmarshal = func([]byte, interface{}) error { return errors.New("jsonUnmarshalError") }
	err = sfForwarder.process([]core.Datapoint{dpSent})
	a.ExpectEquals(t, "jsonUnmarshalError", err.Error(), "Expected ioutil decoding response")
	jsonXXXUnmarshal = json.Unmarshal

	ioutilXXXReadAll = func(r io.Reader) ([]byte, error) { return []byte(`"invalidbody"`), nil }
	err = sfForwarder.process([]core.Datapoint{dpSent})
	a.ExpectContains(t, err.Error(), "Body decode error", "Expected body decoding error")
	ioutilXXXReadAll = ioutil.ReadAll
}