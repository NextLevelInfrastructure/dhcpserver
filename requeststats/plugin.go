// Copyright 2023 Next Level Infrastructure, LLC
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// This plugin exports request statistics to Prometheus

package requeststats

import (
        "github.com/prometheus/client_golang/prometheus"
        "github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv6"
)

var log = logger.GetLogger("plugins/requeststats")

var Plugin = plugins.Plugin{
	Name:   "requeststats",
	Setup6: setup6,
	Setup4: setup4,
}

var (
	v4types = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dhcpv4_requests_total",
		Help: "DHCPv4 requests received, by message type",
	}, []string{"type"})
	v4relay = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dhcpv4_from_relays_total",
		Help: "Total number of DHCPv4 requests recieved from a relay",
	})
	v4raimissingsuboptions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dhcpv4_rai_missing_suboptions_total",
		Help: "DHCPv4 missing Relay Agent Information suboptions in request, by missing suboption",
	}, []string{"suboption"})
	v6types = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dhcpv6_requests_total",
		Help: "DHCPv6 requests received, by message type",
	}, []string{"type"})
	v6relay = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dhcpv6_from_relays_total",
		Help: "Total number of DHCPv6 requests received from a relay",
	})
	v6ia = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dhcpv6_requested_ias_total",
		Help: "DHCPv6 Identity Associations requested, by type {IA_NA, IA_TA, IA_PD}",
	}, []string{"type"})
)

type PluginState struct {
	// we currently have no state; perhaps we might develop some later?
	//sync.Mutex
}

func (state *PluginState) Handler6(req, resp dhcpv6.DHCPv6) (dhcpv6.DHCPv6, bool) {
	if req.IsRelay() {
		v6relay.Inc()
	} else {
		_, ok := req.(*dhcpv6.Message)
		if !ok {
			v6types.WithLabelValues("error").Inc()
			log.Errorf("request message format bug: %v", req)
			return nil, true
		}
	}
	// inner will be the innermost relay message
	innermsg, err := dhcpv6.DecapsulateRelayIndex(req, -1)
	if err != nil {
		v6types.WithLabelValues("error").Inc()
		log.Errorf("could not decapsulate: %v", err)
		return nil, true
	}
	inner, ok := innermsg.(*dhcpv6.RelayMessage)
	if !ok {
		v6types.WithLabelValues("error").Inc()
		log.Errorf("relay message format bug: %v", innermsg)
		return nil, true
	}
	msg, err := inner.GetInnerMessage()
	if err != nil {
		v6types.WithLabelValues("error").Inc()
		log.Errorf("could not decapsulate inner message: %v", err)
		return nil, true
	}
	v6types.WithLabelValues(msg.MessageType.String()).Inc()
	if ianas := len(msg.Options.IANA()); ianas > 0 {
		v6ia.WithLabelValues("IA_NA").Add(float64(ianas))
	}
	if iatas := len(msg.Options.IATA()); iatas > 0 {
		v6ia.WithLabelValues("IA_TA").Add(float64(iatas))
	}
	if iapds := len(msg.Options.IAPD()); iapds > 0 {
		v6ia.WithLabelValues("IA_PD").Add(float64(iapds))
	}
	return resp, false
}

func (state *PluginState) Handler4(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	if req.OpCode != dhcpv4.OpcodeBootRequest {
		v4types.WithLabelValues("ignored").Inc()
		log.Warningf("not a BootRequest, ignoring %d", req.OpCode)
		return resp, false
	}
	v4types.WithLabelValues(req.MessageType().String()).Inc()
	rai := req.RelayAgentInfo()
	giaddr_invalid := len(req.GatewayIPAddr) == 0 || req.GatewayIPAddr.IsUnspecified()
	if rai == nil || giaddr_invalid {
		if rai != nil {
			log.Infof("DHCPv4 request with giaddr but missing RelayAgentInfo: %s", req)
			// not a suboption but we just need to count it somewhere
			v4raimissingsuboptions.WithLabelValues("GatewayIPAddr").Inc()
			// we account for this as a relay request with missing giaddr
			v4relay.Inc()
		} else if !giaddr_invalid {
			log.Infof("DHCPv4 request with RelayAgentInfo but no giaddr: %s", req)
			// an option, not a suboption, but we will count it here
			v4raimissingsuboptions.WithLabelValues("RelayAgentInfo").Inc()
			// we account for this as a relay request with missing RAI
			v4relay.Inc()
		}
		// not a request from a relay so we are done
		return resp, false
	}
	v4relay.Inc()
	if ip := dhcpv4.GetIP(dhcpv4.LinkSelectionSubOption, (*rai).Options); ip == nil {
		v4raimissingsuboptions.WithLabelValues("LinkSelectionSubOption").Inc()
	}
	intfstr := dhcpv4.GetString(dhcpv4.AgentCircuitIDSubOption, (*rai).Options)
	if len(intfstr) == 0 {
		if intfstr = dhcpv4.GetString(dhcpv4.AgentRemoteIDSubOption, (*rai).Options); len(intfstr) == 0 {
			v4raimissingsuboptions.WithLabelValues("AgentIDSubOption").Inc()
		}
	}
	return resp, false
}

func setup6(args ...string) (handler.Handler6, error) {
	var state PluginState
	return state.Handler6, nil
}

func setup4(args ...string) (handler.Handler4, error) {
	var state PluginState
	return state.Handler4, nil
}
