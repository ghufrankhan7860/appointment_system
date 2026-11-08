// Package uap parses User-Agent headers with ua-parser (the uap-core rules used by
// most analytics tools). The rules are compiled into the library, so no data file
// ships with the binary; updating them means updating the module.
package uap

import (
	"fmt"

	"github.com/ua-parser/uap-go/uaparser"

	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

// unknownFamily is what ua-parser returns for a part it does not recognise.
const unknownFamily = "Other"

// Parser is safe for concurrent use: the library guards its cache and
// counters itself.
type Parser struct {
	parser *uaparser.Parser
}

func New() (*Parser, error) {
	parser, err := uaparser.New(uaparser.WithCacheSize(constant.UserAgentCacheSize))
	if err != nil {
		return nil, fmt.Errorf("load user agent rules: %w", err)
	}

	return &Parser{parser: parser}, nil
}

func (p *Parser) Parse(userAgent string) models.UserAgentDetails {
	if userAgent == "" {
		return models.UserAgentDetails{}
	}

	client := p.parser.Parse(userAgent)

	return models.UserAgentDetails{
		BrowserName:    known(client.UserAgent.Family),
		BrowserVersion: version(client.UserAgent.Family, client.UserAgent.Major, client.UserAgent.Minor),
		OSName:         known(client.Os.Family),
		OSVersion:      version(client.Os.Family, client.Os.Major, client.Os.Minor),
		DeviceBrand:    client.Device.Brand,
		DeviceModel:    client.Device.Model,
	}
}

func known(family string) string {
	if family == unknownFamily {
		return ""
	}

	return family
}

func version(family string, major string, minor string) string {
	if family == unknownFamily || major == "" {
		return ""
	}
	if minor == "" {
		return major
	}

	return major + "." + minor
}
