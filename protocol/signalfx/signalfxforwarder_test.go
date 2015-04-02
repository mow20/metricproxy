package signalfx

import (
	"fmt"
	"testing"

	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"runtime/debug"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cep21/gohelpers/workarounds"
	"github.com/golang/protobuf/proto"
	"github.com/signalfx/metricproxy/config"
	"github.com/signalfx/metricproxy/datapoint"
	"github.com/signalfx/metricproxy/nettest"

	"github.com/signalfx/metricproxy/datapoint/dptest"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestBodySendFormat(t *testing.T) {
	b := &BodySendFormatV2{
		Metric: "cpu",
	}
	assert.Contains(t, b.String(), "cpu", "Expect cpu")
}

type metricPanicDatapoint struct {
	datapoint.Datapoint
}

func (vp *metricPanicDatapoint) Metric() string {
	panic("This shouldn't happen!")
}

func TestForwarderLoaderDefaults(t *testing.T) {
	forwardTo := config.ForwardTo{
		FormatVersion:    workarounds.GolangDoesnotAllowPointerToUintLiteral(2),
		DefaultAuthToken: workarounds.GolangDoesnotAllowPointerToStringLiteral("AUTH_TOKEN"),
	}
	ctx := context.Background()
	forwarder, err := ForwarderLoader(ctx, &forwardTo)
	assert.Nil(t, err)
	defer forwarder.Close()
}

func TestMapToDimensions(t *testing.T) {
	r := map[string]string{
		"dim1":     "val1",
		"dim2":     "val2",
		"key:char": "val3",
	}
	res := mapToDimensions(r)
	expect := make(map[string]string)
	for _, d := range res {
		expect[d.GetKey()] = d.GetValue()
	}
	delete(r, "key:char")
	r["key_char"] = "val3"
	assert.Equal(t, r, expect, "Dimensions don't parse right")
	r["invalid_val"] = ""
	res = mapToDimensions(r)
	delete(r, "invalid_val")
	assert.Equal(t, r, expect, "Dimensions don't parse right")
}

func TestFilterSignalfxString(t *testing.T) {
	assert.Equal(t, "hello", filterSignalfxKey("hello"), "Filter not working")
	assert.Equal(t, "_hello_bob1__", filterSignalfxKey(".hello:bob1_&"), "Filter not working")
}

func setupServerForwarder(t *testing.T) (*dptest.BasicSink, *ListenerServer, *Forwarder) {
	// TODO: Break this out into smaller tests
	listenFromSignalfx := config.ListenFrom{}
	listenFromSignalfx.ListenAddr = workarounds.GolangDoesnotAllowPointerToStringLiteral("127.0.0.1:0")

	finalDatapointDestination := dptest.NewBasicSink()
	ctx := context.Background()
	l, err := ListenerLoader(ctx, finalDatapointDestination, &listenFromSignalfx)
	assert.Equal(t, nil, err, "Expect no error")

	port := nettest.TCPPort(l.listener)

	forwardTo := config.ForwardTo{
		URL:              workarounds.GolangDoesnotAllowPointerToStringLiteral(fmt.Sprintf("http://127.0.0.1:%d/v2/datapoint", port)),
		TimeoutDuration:  workarounds.GolangDoesnotAllowPointerToDurationLiteral(time.Second * 1),
		DefaultAuthToken: workarounds.GolangDoesnotAllowPointerToStringLiteral("AUTH_TOKEN"),
		DefaultSource:    workarounds.GolangDoesnotAllowPointerToStringLiteral("proxy-source"),
		SourceDimensions: workarounds.GolangDoesnotAllowPointerToStringLiteral("username,ignored,hostname"),
	}

	_, forwarder, err := ForwarderLoader1(ctx, &forwardTo)
	assert.NoError(t, err, "Expect no error")
	return finalDatapointDestination, l, forwarder
}

func TestDefaultSource(t *testing.T) {
	finalDatapointDestination, l, forwarder := setupServerForwarder(t)
	defer l.Close()

	timeToSend := time.Now().Round(time.Second)
	dpSent := datapoint.New("metric", map[string]string{}, datapoint.NewIntValue(2), datapoint.Gauge, timeToSend)
	go forwarder.AddDatapoints(context.Background(), []*datapoint.Datapoint{dpSent})
	dpRecieved := finalDatapointDestination.Next()
	i := dpRecieved.Value.(datapoint.IntValue).Int()
	assert.Equal(t, int64(2), i, "Expect 2 back")
	assert.Equal(t, "proxy-source", dpRecieved.Dimensions["sf_source"], "Expect ahost back")
	assert.Equal(t, timeToSend, dpRecieved.Timestamp)
}

func TestSetSource(t *testing.T) {
	finalDatapointDestination, l, forwarder := setupServerForwarder(t)
	defer l.Close()

	timeToSend := time.Now().Round(time.Second)
	dpSent := datapoint.New("metric", map[string]string{"cpusize": "big", "hostname": "ahost"}, datapoint.NewIntValue(2), datapoint.Gauge, timeToSend)
	go forwarder.AddDatapoints(context.Background(), []*datapoint.Datapoint{dpSent})
	dpRecieved := finalDatapointDestination.Next()
	i := dpRecieved.Value.(datapoint.IntValue).Int()
	assert.Equal(t, int64(2), i, "Expect 2 back")
	assert.Equal(t, "ahost", dpRecieved.Dimensions["sf_source"], "Expect ahost back")
	assert.Equal(t, timeToSend, dpRecieved.Timestamp)
}

func TestForwarderLoaderOldVersion(t *testing.T) {
	forwardTo := config.ForwardTo{
		FormatVersion:    workarounds.GolangDoesnotAllowPointerToUintLiteral(1),
		DefaultAuthToken: workarounds.GolangDoesnotAllowPointerToStringLiteral("AUTH_TOKEN"),
	}
	ctx := context.Background()
	_, err := ForwarderLoader(ctx, &forwardTo)
	assert.NoError(t, err)
}

func TestNoSource(t *testing.T) {
	finalDatapointDestination, l, forwarder := setupServerForwarder(t)
	defer l.Close()

	forwarder.defaultSource = ""
	timeToSend := time.Now().Round(time.Second)
	dpSent := datapoint.New("metric", map[string]string{}, datapoint.NewIntValue(2), datapoint.Gauge, timeToSend)
	go forwarder.AddDatapoints(context.Background(), []*datapoint.Datapoint{dpSent})
	dpRecieved := finalDatapointDestination.Next()
	i := dpRecieved.Value.(datapoint.IntValue).Int()
	assert.Equal(t, 2, i, "Expect 2 back")
	val, exists := dpRecieved.Dimensions["sf_source"]
	assert.False(t, exists, val)
}

func TestDatumForPoint(t *testing.T) {
	assert.Equal(t, 3, datumForPoint(datapoint.NewIntValue(3)).GetIntValue())
	assert.Equal(t, 0.0, datumForPoint(datapoint.NewIntValue(3)).GetDoubleValue())
	assert.Equal(t, .1, datumForPoint(datapoint.NewFloatValue(.1)).GetDoubleValue())
	assert.Equal(t, "hi", datumForPoint(datapoint.NewStringValue("hi")).GetStrValue())
}

func TestConnectorProcessProtoError(t *testing.T) {
	f := Forwarder{
		protoMarshal: func(pb proto.Message) ([]byte, error) {
			return nil, fmt.Errorf("marshal error")
		},
	}
	err := f.AddDatapoints(context.Background(), []*datapoint.Datapoint{})
	assert.Equal(t, "marshal error", err.Error())
}

type roundTripTest func(r *http.Request) (*http.Response, error)

func (r roundTripTest) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

func TestClientReqError(t *testing.T) {
	f := Forwarder{
		protoMarshal: proto.Marshal,
		client: &http.Client{
			Transport: roundTripTest(func(r *http.Request) (*http.Response, error) {
				debug.PrintStack()
				return nil, fmt.Errorf("unable to execute http request")
			}),
		},
	}
	err := f.AddDatapoints(context.Background(), []*datapoint.Datapoint{})
	assert.Contains(t, err.Error(), "unable to execute http request")
}

type readError struct {
}

func (r *readError) Read([]byte) (int, error) {
	return 0, fmt.Errorf("read error")
}

func TestResponseBodyError(t *testing.T) {
	f := Forwarder{
		protoMarshal: proto.Marshal,
		client: &http.Client{
			Transport: roundTripTest(func(r *http.Request) (*http.Response, error) {
				r2 := http.Response{
					Body: ioutil.NopCloser(&readError{}),
				}
				return &r2, nil
			}),
		},
	}
	err := f.AddDatapoints(context.Background(), []*datapoint.Datapoint{})
	assert.Equal(t, "read error", err.Error())
}

func TestResponseBadStatus(t *testing.T) {
	f := Forwarder{
		protoMarshal: proto.Marshal,
		client: &http.Client{
			Transport: roundTripTest(func(r *http.Request) (*http.Response, error) {
				r2 := http.Response{
					Body:       ioutil.NopCloser(bytes.NewBufferString("")),
					StatusCode: 404,
				}
				return &r2, nil
			}),
		},
	}
	err := f.AddDatapoints(context.Background(), []*datapoint.Datapoint{})
	assert.Contains(t, err.Error(), "invalid status code")
}

func TestResponseBadJSON(t *testing.T) {
	f := Forwarder{
		protoMarshal: proto.Marshal,
		client: &http.Client{
			Transport: roundTripTest(func(r *http.Request) (*http.Response, error) {
				r2 := http.Response{
					Body:       ioutil.NopCloser(bytes.NewBufferString("INVALID_JSON")),
					StatusCode: 200,
				}
				return &r2, nil
			}),
		},
	}
	err := f.AddDatapoints(context.Background(), []*datapoint.Datapoint{})
	assert.IsType(t, &json.SyntaxError{}, err)
}

func TestResponseBadBody(t *testing.T) {
	f := Forwarder{
		protoMarshal: proto.Marshal,
		client: &http.Client{
			Transport: roundTripTest(func(r *http.Request) (*http.Response, error) {
				r2 := http.Response{
					Body:       ioutil.NopCloser(bytes.NewBufferString(`"BAD"`)),
					StatusCode: 200,
				}
				return &r2, nil
			}),
		},
	}
	err := f.AddDatapoints(context.Background(), []*datapoint.Datapoint{})
	assert.Contains(t, err.Error(), "body decode error")
}
