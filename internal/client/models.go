package client

import (
	"encoding/json"
	"strconv"
)

// normalizeNumeric coerces the string form NodePing sometimes uses for numeric
// fields ("5", "300") into a float64, so downstream code sees exactly one type
// regardless of which representation the API happened to return.
//
// Values that are not numeric-looking are returned untouched.
func normalizeNumeric(v interface{}) interface{} {
	switch t := v.(type) {
	case string:
		if t == "" {
			return v
		}
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f
		}
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f
		}
	case int:
		return float64(t)
	case int64:
		return float64(t)
	}
	return v
}

// numberFromJSON decodes a JSON value that may be either a number or a
// numeric string into a json.Number.
func numberFromJSON(raw json.RawMessage) (json.Number, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return "", false
		}
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			return "", false
		}
		return json.Number(s), true
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	return "", false
}

type Contact struct {
	ID         string           `json:"_id,omitempty"`
	Type       string           `json:"type,omitempty"`
	CustomerID string           `json:"customer_id,omitempty"`
	Name       string           `json:"name,omitempty"`
	CustRole   string           `json:"custrole,omitempty"`
	Addresses  ContactAddresses `json:"addresses,omitempty"`
}

type ContactAddress struct {
	Address string `json:"address,omitempty"`
	Type    string `json:"type,omitempty"`
	Status  string `json:"status,omitempty"`
	// The suppress* flags deliberately omit `omitempty`. They are sent on the
	// update path, and with omitempty a value flipped back to false would
	// marshal to nothing, leaving NodePing's stored `true` in place forever.
	SuppressUp    bool `json:"suppressup"`
	SuppressDown  bool `json:"suppressdown"`
	SuppressFirst bool `json:"suppressfirst"`
	SuppressDiag  bool `json:"suppressdiag"`
	SuppressAll   bool `json:"suppressall"`
	// Mute keeps omitempty: a nil RawMessage means "not specified". Callers
	// that want to clear a mute must send an explicit []byte("false") rather
	// than leaving the field nil.
	Mute         json.RawMessage `json:"mute,omitempty"`
	Action       string          `json:"action,omitempty"`
	Headers      FlexStringMap   `json:"headers,omitempty"`
	QueryStrings FlexStringMap   `json:"querystrings,omitempty"`
	Data         interface{}     `json:"data,omitempty"`
	Priority     *int            `json:"priority,omitempty"`
}

type NewAddress struct {
	Address       string `json:"address"`
	Type          string `json:"type"`
	SuppressUp    bool   `json:"suppressup"`
	SuppressDown  bool   `json:"suppressdown"`
	SuppressFirst bool   `json:"suppressfirst"`
	SuppressDiag  bool   `json:"suppressdiag"`
	SuppressAll   bool   `json:"suppressall"`
	// Mute is interface{}: encoding/json only treats a nil interface as empty,
	// so an explicit `false` is still transmitted.
	Mute         interface{}   `json:"mute,omitempty"`
	Action       string        `json:"action,omitempty"`
	Headers      FlexStringMap `json:"headers,omitempty"`
	QueryStrings FlexStringMap `json:"querystrings,omitempty"`
	Data         string        `json:"data,omitempty"`
	Priority     *int          `json:"priority,omitempty"`
}

type ContactCreateRequest struct {
	Name         string       `json:"name,omitempty"`
	CustRole     string       `json:"custrole,omitempty"`
	NewAddresses []NewAddress `json:"newaddresses,omitempty"`
}

type ContactUpdateRequest struct {
	Name     string `json:"name,omitempty"`
	CustRole string `json:"custrole,omitempty"`
	// Addresses deliberately omits `omitempty`. NodePing treats the map as the
	// complete address set and deletes anything left out of it, so an update
	// that moves every address into newaddresses has to send an explicit `{}`
	// or the old addresses survive alongside the new ones. A nil map still
	// marshals to `null`, which NodePing reads as "not specified" and leaves
	// the addresses alone; `{}` is refused outright when it would empty the
	// contact.
	Addresses    map[string]ContactAddress `json:"addresses"`
	NewAddresses []NewAddress              `json:"newaddresses,omitempty"`
}

type Check struct {
	ID            string                   `json:"_id,omitempty"`
	Rev           string                   `json:"_rev,omitempty"`
	CustomerID    string                   `json:"customer_id,omitempty"`
	Label         string                   `json:"label,omitempty"`
	Type          string                   `json:"type,omitempty"`
	Interval      json.Number              `json:"interval,omitempty"`
	Enabled       string                   `json:"enable,omitempty"`
	Public        bool                     `json:"public,omitempty"`
	Status        string                   `json:"status,omitempty"`
	Modified      int64                    `json:"modified,omitempty"`
	Created       int64                    `json:"created,omitempty"`
	State         int                      `json:"state,omitempty"`
	FirstDown     interface{}              `json:"firstdown,omitempty"`
	Notifications []map[string]interface{} `json:"notifications,omitempty"`
	Parameters    CheckParameters          `json:"parameters,omitempty"`
	Dep           interface{}              `json:"dep,omitempty"`
	Mute          interface{}              `json:"mute,omitempty"`
	Description   string                   `json:"description,omitempty"`
	UUID          string                   `json:"uuid,omitempty"`
	RunLocations  interface{}              `json:"runlocations,omitempty"`
	HomeLoc       interface{}              `json:"homeloc,omitempty"`
	AutoDiag      bool                     `json:"autodiag,omitempty"`
	Tags          []string                 `json:"tags,omitempty"`
}

// UnmarshalJSON tolerates NodePing returning `interval` as either a JSON
// number or a numeric string. json.Number alone rejects the string form, which
// would fail the decode of the entire check.
func (c *Check) UnmarshalJSON(data []byte) error {
	type checkAlias Check
	aux := struct {
		Interval json.RawMessage `json:"interval,omitempty"`
		*checkAlias
	}{
		checkAlias: (*checkAlias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if n, ok := numberFromJSON(aux.Interval); ok {
		c.Interval = n
	} else {
		c.Interval = ""
	}
	return nil
}

type CheckParameters struct {
	Target         string                `json:"target,omitempty"`
	Threshold      interface{}           `json:"threshold,omitempty"`
	Sens           interface{}           `json:"sens,omitempty"`
	ContentString  string                `json:"contentstring,omitempty"`
	Regex          interface{}           `json:"regex,omitempty"`
	Invert         interface{}           `json:"invert,omitempty"`
	Follow         interface{}           `json:"follow,omitempty"`
	Method         string                `json:"method,omitempty"`
	StatusCode     interface{}           `json:"statuscode,omitempty"`
	SendHeaders    map[string]string     `json:"sendheaders,omitempty"`
	ReceiveHeaders map[string]string     `json:"receiveheaders,omitempty"`
	Data           interface{}           `json:"data,omitempty"`
	PostData       string                `json:"postdata,omitempty"`
	Port           interface{}           `json:"port,omitempty"`
	Username       string                `json:"username,omitempty"`
	Password       string                `json:"password,omitempty"`
	Secure         interface{}           `json:"secure,omitempty"`
	Verify         interface{}           `json:"verify,omitempty"`
	IPv6           interface{}           `json:"ipv6,omitempty"`
	DNSType        string                `json:"dnstype,omitempty"`
	DNSToResolve   string                `json:"dnstoresolve,omitempty"`
	DNSSection     string                `json:"dnssection,omitempty"`
	DNSRD          interface{}           `json:"dnsrd,omitempty"`
	Transport      string                `json:"transport,omitempty"`
	WarningDays    interface{}           `json:"warningdays,omitempty"`
	ServerName     string                `json:"servername,omitempty"`
	Email          string                `json:"email,omitempty"`
	Database       string                `json:"database,omitempty"`
	Query          string                `json:"query,omitempty"`
	Namespace      string                `json:"namespace,omitempty"`
	Fields         map[string]CheckField `json:"fields,omitempty"`
	Hosts          map[string]RedisHost  `json:"hosts,omitempty"`
	RedisType      string                `json:"redistype,omitempty"`
	SentinelName   string                `json:"sentinelname,omitempty"`
	SSHKey         interface{}           `json:"sshkey,omitempty"`
	ClientCert     interface{}           `json:"clientcert,omitempty"`
	CheckToken     string                `json:"checktoken,omitempty"`
	OldResultFail  interface{}           `json:"oldresultfail,omitempty"`
	Ignore         string                `json:"ignore,omitempty"`
	DoHDoT         string                `json:"dohdot,omitempty"`
	EDNS           map[string]string     `json:"edns,omitempty"`
	WhoisServer    string                `json:"whoisserver,omitempty"`
	RDAPUrl        string                `json:"rdapurl,omitempty"`
	SNMPv          string                `json:"snmpv,omitempty"`
	SNMPCom        string                `json:"snmpcom,omitempty"`
	VerifyVolume   interface{}           `json:"verifyvolume,omitempty"`
	VolumeMin      interface{}           `json:"volumemin,omitempty"`
}

// UnmarshalJSON normalizes the numeric parameters that NodePing returns as
// strings in some responses and as numbers in others. Downstream code
// type-asserts these to float64, so a string would silently map to a null
// attribute in Terraform state.
func (p *CheckParameters) UnmarshalJSON(data []byte) error {
	type checkParametersAlias CheckParameters
	var aux checkParametersAlias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*p = CheckParameters(aux)

	p.Threshold = normalizeNumeric(p.Threshold)
	p.Sens = normalizeNumeric(p.Sens)
	p.Port = normalizeNumeric(p.Port)
	p.WarningDays = normalizeNumeric(p.WarningDays)
	p.StatusCode = normalizeNumeric(p.StatusCode)
	p.VolumeMin = normalizeNumeric(p.VolumeMin)

	return nil
}

type CheckField struct {
	Name  string      `json:"name,omitempty"`
	Min   interface{} `json:"min,omitempty"`
	Max   interface{} `json:"max,omitempty"`
	Match string      `json:"match,omitempty"`
}

type RedisHost struct {
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"`
	Password string `json:"password,omitempty"`
}

type CheckCreateRequest struct {
	Type          string                   `json:"type"`
	Target        string                   `json:"target,omitempty"`
	Label         string                   `json:"label,omitempty"`
	Interval      interface{}              `json:"interval,omitempty"`
	Enabled       string                   `json:"enabled,omitempty"`
	Public        interface{}              `json:"public,omitempty"`
	AutoDiag      interface{}              `json:"autodiag,omitempty"`
	RunLocations  interface{}              `json:"runlocations,omitempty"`
	HomeLoc       interface{}              `json:"homeloc,omitempty"`
	Threshold     interface{}              `json:"threshold,omitempty"`
	Sens          interface{}              `json:"sens,omitempty"`
	Notifications []map[string]interface{} `json:"notifications,omitempty"`
	Dep           interface{}              `json:"dep,omitempty"`
	Mute          interface{}              `json:"mute,omitempty"`
	Description   string                   `json:"description,omitempty"`
	// Tags is a pointer so that clearing every tag is transmissible. A plain
	// []string with omitempty drops an empty slice, making "remove the last
	// tag" a silent no-op. nil = leave unchanged; &[]string{} = clear all.
	Tags           *[]string             `json:"tags,omitempty"`
	ContentString  string                `json:"contentstring,omitempty"`
	Regex          interface{}           `json:"regex,omitempty"`
	Invert         interface{}           `json:"invert,omitempty"`
	Follow         interface{}           `json:"follow,omitempty"`
	Method         string                `json:"method,omitempty"`
	StatusCode     interface{}           `json:"statuscode,omitempty"`
	SendHeaders    map[string]string     `json:"sendheaders,omitempty"`
	ReceiveHeaders map[string]string     `json:"receiveheaders,omitempty"`
	Data           interface{}           `json:"data,omitempty"`
	PostData       string                `json:"postdata,omitempty"`
	Port           interface{}           `json:"port,omitempty"`
	Username       string                `json:"username,omitempty"`
	Password       string                `json:"password,omitempty"`
	Secure         interface{}           `json:"secure,omitempty"`
	Verify         interface{}           `json:"verify,omitempty"`
	IPv6           interface{}           `json:"ipv6,omitempty"`
	DNSType        string                `json:"dnstype,omitempty"`
	DNSToResolve   string                `json:"dnstoresolve,omitempty"`
	DNSSection     string                `json:"dnssection,omitempty"`
	DNSRD          interface{}           `json:"dnsrd,omitempty"`
	Transport      string                `json:"transport,omitempty"`
	WarningDays    interface{}           `json:"warningdays,omitempty"`
	ServerName     string                `json:"servername,omitempty"`
	Email          string                `json:"email,omitempty"`
	Database       string                `json:"database,omitempty"`
	Query          string                `json:"query,omitempty"`
	Namespace      string                `json:"namespace,omitempty"`
	Fields         map[string]CheckField `json:"fields,omitempty"`
	Hosts          map[string]RedisHost  `json:"hosts,omitempty"`
	RedisType      string                `json:"redistype,omitempty"`
	SentinelName   string                `json:"sentinelname,omitempty"`
	SSHKey         string                `json:"sshkey,omitempty"`
	ClientCert     string                `json:"clientcert,omitempty"`
	CheckToken     string                `json:"checktoken,omitempty"`
	OldResultFail  interface{}           `json:"oldresultfail,omitempty"`
	Ignore         string                `json:"ignore,omitempty"`
	DoHDoT         string                `json:"dohdot,omitempty"`
	EDNS           map[string]string     `json:"edns,omitempty"`
	WhoisServer    string                `json:"whoisserver,omitempty"`
	RDAPUrl        string                `json:"rdapurl,omitempty"`
	SNMPv          string                `json:"snmpv,omitempty"`
	SNMPCom        string                `json:"snmpcom,omitempty"`
	VerifyVolume   interface{}           `json:"verifyvolume,omitempty"`
	VolumeMin      interface{}           `json:"volumemin,omitempty"`
}

type CheckUpdateRequest struct {
	CheckCreateRequest
}

type Notification struct {
	Delay    int    `json:"delay"`
	Schedule string `json:"schedule"`
}

type DeleteResponse struct {
	OK bool   `json:"ok"`
	ID string `json:"id"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
