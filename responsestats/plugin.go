// Copyright 2023 Next Level Infrastructure, LLC
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// This plugin exports response statistics to Prometheus

package responsestats

import (
	"bytes"
	"fmt"

        "github.com/prometheus/client_golang/prometheus"
        "github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/iana"
)

var log = logger.GetLogger("plugins/responsestats")

var Plugin = plugins.Plugin{
	Name:   "responsestats",
	Setup6: setup6,
	Setup4: setup4,
}

var (
	v4types = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dhcpv4_responses_total",
		Help: "DHCPv4 responses sent, by message type",
	}, []string{"type"})
	v4processed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dhcpv4_leases_processed_total",
		Help: "DHCPv4 leases processed, by result {all, none}",
	}, []string{"result"})
	v4relay = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dhcpv4_to_relays_total",
		Help: "Total number of DHCPv4 responses sent to a relay",
	})
	v6types = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dhcpv6_responses_total",
		Help: "DHCPv6 responses sent, by message type",
	}, []string{"type"})
	v6relay = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dhcpv6_to_relays_total",
		Help: "Total number of DHCPv6 responses sent to a relay",
	})
	v6processed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dhcpv6_ias_processed_total",
		Help: "DHCPv6 Identity Associations processed, by type {IA_NA, IA_TA, IA_PD} X result {all, some, none}",
	}, []string{"type", "result"})
)

type OptionCode = dhcpv6.OptionCode

type IdentityAssociation interface {
	Id()        [4]byte
	Code()      OptionCode
	ToBytes()   []byte
	String()    string
	New([4]byte) IdentityAssociation
	Allocated() bool
	AddStatusUnavailable()
}

type OptIANA dhcpv6.OptIANA
type OptIATA dhcpv6.OptIATA
type OptIAPD dhcpv6.OptIAPD

func FromIANA(ia []*dhcpv6.OptIANA) []IdentityAssociation {
	converted := make([]IdentityAssociation, len(ia))
	for idx, iana := range ia {
		converted[idx] = (*OptIANA)(iana)
	}
	return converted
}

func FromIATA(ia []*dhcpv6.OptIATA) []IdentityAssociation {
	converted := make([]IdentityAssociation, len(ia))
	for idx, iata := range ia {
		converted[idx] = (*OptIATA)(iata)
	}
	return converted
}

func FromIAPD(ia []*dhcpv6.OptIAPD) []IdentityAssociation {
	converted := make([]IdentityAssociation, len(ia))
	for idx, iapd := range ia {
		converted[idx] = (*OptIAPD)(iapd)
	}
	return converted
}

func (ia *OptIANA) Id() [4]byte { return ia.IaId }
func (ia *OptIATA) Id() [4]byte { return ia.IaId }
func (ia *OptIAPD) Id() [4]byte { return ia.IaId }
func (ia *OptIANA) Code() OptionCode { return dhcpv6.OptionIANA }
func (ia *OptIATA) Code() OptionCode { return dhcpv6.OptionIATA }
func (ia *OptIAPD) Code() OptionCode { return dhcpv6.OptionIAPD }
func (ia *OptIANA) ToBytes() []byte { return (*(*dhcpv6.OptIANA)(ia)).ToBytes() }
func (ia *OptIATA) ToBytes() []byte { return (*(*dhcpv6.OptIATA)(ia)).ToBytes() }
func (ia *OptIAPD) ToBytes() []byte { return (*(*dhcpv6.OptIAPD)(ia)).ToBytes() }
func (ia *OptIANA) String() string { return (*(*dhcpv6.OptIANA)(ia)).String() }
func (ia *OptIATA) String() string { return (*(*dhcpv6.OptIATA)(ia)).String() }
func (ia *OptIAPD) String() string { return (*(*dhcpv6.OptIAPD)(ia)).String() }
func (ia *OptIANA) New(iaid [4]byte) IdentityAssociation { return &OptIANA{IaId: iaid} }
func (ia *OptIATA) New(iaid [4]byte) IdentityAssociation { return &OptIATA{IaId: iaid} }
func (ia *OptIAPD) New(iaid [4]byte) IdentityAssociation { return &OptIAPD{IaId: iaid} }
func (ia *OptIANA) Allocated() bool {return (*(*dhcpv6.OptIANA)(ia)).Options.OneAddress() != nil }
func (ia *OptIATA) Allocated() bool {return (*(*dhcpv6.OptIATA)(ia)).Options.OneAddress() != nil }
func (ia *OptIAPD) Allocated() bool {return len((*(*dhcpv6.OptIAPD)(ia)).Options.Prefixes()) > 0 }
func (ia *OptIANA) AddStatusUnavailable() {
	(*(*dhcpv6.OptIANA)(ia)).Options.Add(&dhcpv6.OptStatusCode{StatusCode: iana.StatusNoAddrsAvail})
}
func (ia *OptIATA) AddStatusUnavailable() {
	(*(*dhcpv6.OptIATA)(ia)).Options.Add(&dhcpv6.OptStatusCode{StatusCode: iana.StatusNoAddrsAvail})
}
func (ia *OptIAPD) AddStatusUnavailable() {
	(*(*dhcpv6.OptIAPD)(ia)).Options.Add(&dhcpv6.OptStatusCode{StatusCode: iana.StatusNoPrefixAvail})
}

type StringLogger func(string)

type PluginState struct {
	//sync.Mutex
	Logger StringLogger
}

func ia_fixup(resp *dhcpv6.DHCPv6, request_ias, response_ias []IdentityAssociation) (string, int) {
	satisfied := 0
	unsatisfied := 0
	newstatus := 0
	for _, reqia := range request_ias {
		found := false
		iaid := reqia.Id()
		for _, respia := range response_ias {
			respiaid := respia.Id()
			if bytes.Compare(iaid[:], respiaid[:]) == 0 {
				if respia.Allocated() {
					satisfied++
				} else {
					unsatisfied++
				}
				found = true
				break
			}
		}
		if !found {
			unsatisfied++
			newstatus++
			newresp := reqia.New(iaid)
			newresp.AddStatusUnavailable()
			(*resp).AddOption(newresp)
		}
	}
	if unsatisfied == 0 {
		return "all", newstatus
	} else if satisfied == 0 {
		return "none", newstatus
	}
	return "some", newstatus
}

func (state *PluginState) Handler6(req, resp dhcpv6.DHCPv6) (dhcpv6.DHCPv6, bool) {
	respmsg, ok := resp.(*dhcpv6.Message)
	if !ok {
		v6types.WithLabelValues("error").Inc()
		log.Errorf("response message format bug: %v", respmsg)
		return nil, true
	}
	reqmsg, ok := req.(*dhcpv6.Message)
	if !ok {
		v6types.WithLabelValues("error").Inc()
		log.Errorf("request message format bug: %v", respmsg)
		return nil, true
	}
	if reqmsg.IsRelay() {
		v6relay.Inc()
	}
	v6types.WithLabelValues(respmsg.MessageType.String()).Inc()
	all_adds := 0
	if len(reqmsg.Options.IANA()) > 0 {
		quantifier, adds := ia_fixup(&resp, FromIANA(reqmsg.Options.IANA()), FromIANA(respmsg.Options.IANA()))
		v6processed.WithLabelValues("IA_NA", quantifier).Inc()
		all_adds = all_adds + adds
	}
	if len(reqmsg.Options.IATA()) > 0 {
		quantifier, adds := ia_fixup(&resp, FromIATA(reqmsg.Options.IATA()), FromIATA(respmsg.Options.IATA()))
		v6processed.WithLabelValues("IA_TA", quantifier).Inc()
		all_adds = all_adds + adds
	}
	if len(reqmsg.Options.IAPD()) > 0 {
		quantifier, adds := ia_fixup(&resp, FromIAPD(reqmsg.Options.IAPD()), FromIAPD(respmsg.Options.IAPD()))
		v6processed.WithLabelValues("IA_PD", quantifier).Inc()
		all_adds = all_adds + adds
	}
	if all_adds > 0 {
		state.Logger(fmt.Sprintf("[added %d statuscodes] %s", all_adds, resp))
	} else {
		state.Logger(resp.String())
	}
	return resp, false
}

func (state *PluginState) Handler4(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	if req.OpCode != dhcpv4.OpcodeBootRequest {
		return resp, false
	}
	mac := req.ClientHWAddr
	has_yiaddr := len(resp.YourIPAddr) > 0 && !resp.YourIPAddr.IsUnspecified()
	if resp.MessageType() == dhcpv4.MessageTypeAck && has_yiaddr {
		v4processed.WithLabelValues("all").Inc()
	}
	v4types.WithLabelValues(resp.MessageType().String()).Inc()
	rai := req.RelayAgentInfo()
	if rai == nil {
		// not a relay message
		if len(resp.GatewayIPAddr) == 0 || resp.GatewayIPAddr.IsUnspecified() {
			state.Logger(fmt.Sprintf("MAC %s allocated %s", mac, req.YourIPAddr))
		} else {
			state.Logger(fmt.Sprintf("[giaddr=%s has no RAI] MAC %s allocated %s", resp.GatewayIPAddr, mac, req.YourIPAddr))
		}
		return resp, false
	}
	v4relay.Inc()
	peerstr := req.GatewayIPAddr.String()
	var linkstr string
	if ip := dhcpv4.GetIP(dhcpv4.LinkSelectionSubOption, (*rai).Options); ip != nil {
		linkstr = ip.String()
	}
	intfstr := dhcpv4.GetString(dhcpv4.AgentCircuitIDSubOption, (*rai).Options)
	if len(intfstr) == 0 {
		if intfstr = dhcpv4.GetString(dhcpv4.AgentRemoteIDSubOption, (*rai).Options); len(intfstr) == 0 {
			intfstr = "<unspecified>"
		}
	}
	state.Logger(fmt.Sprintf("[relay=%s link=%s intf=%s] MAC %s allocated %s", peerstr, linkstr, intfstr, mac, req.YourIPAddr))

	return resp, false
}

func setup6(args ...string) (handler.Handler6, error) {
	var state PluginState
	if err := state.FromArgs(args...); err != nil {
		return nil, err
	}
	return state.Handler6, nil
}

func setup4(args ...string) (handler.Handler4, error) {
	var state PluginState
	if err := state.FromArgs(args...); err != nil {
		return nil, err
	}
	return state.Handler4, nil
}

func (state *PluginState) FromArgs(args ...string) error {
	if len(args) > 0 && args[0] == "silent" {
		state.Logger = func (s string) {
			log.Debug(s)
		}
	} else {
		state.Logger = func (s string) {
			log.Info(s)
		}
	}
	return nil
}
