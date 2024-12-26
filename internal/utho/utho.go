package utho

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mehrdadep/dex"
	log "github.com/sirupsen/logrus"
	"github.com/uthoplatforms/utho-go/utho"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

const (
	uthoCreate = "CREATE"
	uthoDelete = "DELETE"
	uthoUpdate = "UPDATE"
	uthoTTL    = 3600
)

// UthoProvider is the main provider structure implementing the provider interface.
type UthoProvider struct {
	provider.BaseProvider
	client utho.Client

	zoneIDNameMapper provider.ZoneIDName
	domainFilter     endpoint.DomainFilter
	DryRun           bool
}

// UthoChanges represents a change (CREATE, UPDATE, DELETE) to DNS records.
type UthoChanges struct {
	Action string

	ResourceRecordSet utho.CreateDnsRecordParams
}

// Configuration contains the Utho provider's configuration details.
type Configuration struct {
	APIKey               string   `env:"UTHO_API_KEY" required:"true"`
	DryRun               bool     `env:"DRY_RUN" default:"false"`
	DomainFilter         []string `env:"DOMAIN_FILTER" default:""`
	ExcludeDomains       []string `env:"EXCLUDE_DOMAIN_FILTER" default:""`
	RegexDomainFilter    string   `env:"REGEXP_DOMAIN_FILTER" default:""`
	RegexDomainExclusion string   `env:"REGEXP_DOMAIN_FILTER_EXCLUSION" default:""`
}

// NewProvider initializes a new instance of UthoProvider with the given configuration.
func NewProvider(providerConfig *Configuration) (*UthoProvider, error) {
	log.Infof("Creating new provider with API key: %s", providerConfig.APIKey)
	uthoClient, _ := utho.NewClient(providerConfig.APIKey)

	return &UthoProvider{
		client:       uthoClient,
		DryRun:       providerConfig.DryRun,
		domainFilter: GetDomainFilter(*providerConfig),
	}, nil
}

// Zones returns a list of hosted zones.
func (p *UthoProvider) Zones(ctx context.Context) ([]utho.Domain, error) {
	log.Info("Fetching zones")
	zones, err := p.fetchZones()
	if err != nil {
		log.Errorf("Error fetching zones: %v", err)
		return nil, err
	}

	log.Infof("Fetched zones: %v", zones)
	return zones, nil
}

// Records retrieves the list of DNS records for all zones.
func (p *UthoProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	log.Info("Fetching records")
	zones, err := p.Zones(ctx)
	if err != nil {
		log.Errorf("Error fetching zones: %v", err)
		return nil, err
	}

	var endpoints []*endpoint.Endpoint

	for _, zone := range zones {
		log.Infof("Fetching records for zone: %s", zone.Domain)
		records, err := p.client.Domain().ListDnsRecords(zone.Domain)
		if err != nil {
			log.Errorf("Error fetching records for zone %s: %v", zone.Domain, err)
			return nil, err
		}

		for _, r := range records {
			log.Debugf("Processing record: %+v", r)
			// Check if the record type is supported before processing it.
			if provider.SupportedRecordType(r.Type) {
				name := fmt.Sprintf("%s.%s", r.Hostname, zone.Domain)

				// Handle cases where hostname is empty or denotes the root domain.
				if (r.Hostname == "" || r.Hostname == "@") && zone.Domain != "" {
					name = zone.Domain
				}

				parsedTTL, err := strconv.Atoi(r.TTL)
				if err != nil {
					log.Errorf("Invalid TTL value: %s, error: %v", r.TTL, err)
					return nil, fmt.Errorf("invalid TTL value: %w", err)
				}
				endpoints = append(endpoints,
					endpoint.NewEndpointWithTTL(name, r.Type, endpoint.TTL(int64(parsedTTL)), r.Value))
			}
		}
	}

	log.Infof("Fetched endpoints: %v", endpoints)
	return endpoints, nil
}

// fetchRecords retrieves DNS records for a specific domain.
func (p *UthoProvider) fetchRecords(domain string) ([]utho.DnsRecord, error) {
	log.Infof("Fetching records for domain: %s", domain)
	records, err := p.client.Domain().ListDnsRecords(domain)
	if err != nil {
		log.Errorf("Error fetching records for domain %s: %v", domain, err)
		return nil, err
	}

	log.Debugf("Fetched records: %v", records)
	return records, nil
}

// fetchZones retrieves all zones managed by the provider and filters them using the domain filter.
func (p *UthoProvider) fetchZones() ([]utho.Domain, error) {
	log.Info("Fetching all domains")
	var zones []utho.Domain

	allDomains, err := p.client.Domain().ListDomains()
	if err != nil {
		log.Errorf("Error fetching all domains: %v", err)
		return nil, err
	}

	for _, domain := range allDomains {
		log.Debugf("Processing domain: %s", domain.Domain)
		if p.domainFilter.Match(domain.Domain) {
			zones = append(zones, domain)
		}
	}

	log.Infof("Filtered zones: %v", zones)
	return zones, nil
}

// submitChanges processes DNS changes such as CREATE, UPDATE, or DELETE actions.
func (p *UthoProvider) submitChanges(ctx context.Context, changes []*UthoChanges) error {
	log.Infof("Submitting changes: %v", changes)
	if len(changes) == 0 {
		log.Infof("No changes to submit")
		return nil
	}

	zones, err := p.Zones(ctx)
	if err != nil {
		log.Errorf("Error fetching zones during submit: %v", err)
		return err
	}

	zoneChanges := separateChangesByZone(zones, changes)
	cache := "/tmp/list.cache"
	extract, _ := dex.New(cache)

	for zoneName, changes := range zoneChanges {
		log.Infof("Processing changes for zone: %s", zoneName)
		for _, change := range changes {
			log.WithFields(log.Fields{
				"record": change.ResourceRecordSet.Hostname,
				"type":   change.ResourceRecordSet.Type,
				"ttl":    change.ResourceRecordSet.TTL,
				"action": change.Action,
				"zone":   zoneName,
			}).Info("Processing change")

			change.ResourceRecordSet.Domain = zoneName

			// record on the apex domain
			if change.ResourceRecordSet.Hostname == zoneName {
				change.ResourceRecordSet.Hostname = "@"
			} else {
				change.ResourceRecordSet.Hostname = extract.Parse(change.ResourceRecordSet.Hostname).Subdomain
			}

			// Perform the required action (CREATE, UPDATE, DELETE).
			switch change.Action {
			case uthoCreate:
				log.Infof("Creating record: %+v", change.ResourceRecordSet)
				if _, err := p.client.Domain().CreateDnsRecord(change.ResourceRecordSet); err != nil {
					log.Errorf("Error creating record: %v", err)
					return err
				}
			case uthoDelete:
				log.Infof("Deleting record: %+v", change.ResourceRecordSet)
				id, err := p.getRecordID(zoneName, change.ResourceRecordSet)
				if err != nil {
					log.Errorf("Error getting record ID: %v", err)
					return err
				}

				if _, err := p.client.Domain().DeleteDnsRecord(zoneName, id); err != nil {
					log.Errorf("Error deleting record: %v", err)
					return err
				}
			case uthoUpdate:
				log.Infof("Updating record: %+v", change.ResourceRecordSet)
				id, err := p.getRecordID(zoneName, change.ResourceRecordSet)
				if err != nil {
					log.Errorf("Error getting record ID for update: %v", err)
					return err
				}

				// Delete the old record before creating the updated one.
				log.Infof("Deleting old record for update: ID=%s", id)
				if _, err := p.client.Domain().DeleteDnsRecord(zoneName, id); err != nil {
					log.Errorf("Error deleting old record: %v", err)
					return err
				}

				log.Infof("Creating updated record: %+v", change.ResourceRecordSet)
				if _, err := p.client.Domain().CreateDnsRecord(change.ResourceRecordSet); err != nil {
					log.Errorf("Error creating updated record: %v", err)
					return err
				}
			}
		}
	}
	return nil
}

// ApplyChanges consolidates changes and applies them to the DNS records.
func (p *UthoProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	log.Infof("Applying changes: %v", changes)
	combinedChanges := make([]*UthoChanges, 0, len(changes.Create)+len(changes.UpdateNew)+len(changes.Delete))

	// Append CREATE, UPDATE, DELETE changes to a unified list.
	combinedChanges = append(combinedChanges, newUthoChanges(uthoCreate, changes.Create)...)
	combinedChanges = append(combinedChanges, newUthoChanges(uthoUpdate, changes.UpdateNew)...)
	combinedChanges = append(combinedChanges, newUthoChanges(uthoDelete, changes.Delete)...)

	return p.submitChanges(ctx, combinedChanges)
}

// newUthoChanges constructs UthoChanges from a list of endpoints.
func newUthoChanges(action string, endpoints []*endpoint.Endpoint) []*UthoChanges {
	log.Infof("Creating new Utho changes: action=%s, endpoints=%v", action, endpoints)
	changes := make([]*UthoChanges, 0, len(endpoints))
	ttl := uthoTTL
	for _, e := range endpoints {
		// Use custom TTL if configured, otherwise use default.
		if e.RecordTTL.IsConfigured() {
			ttl = int(e.RecordTTL)
		}

		log.Debugf("Processing endpoint: %v", e)
		change := &UthoChanges{
			Action: action,
			ResourceRecordSet: utho.CreateDnsRecordParams{
				Type:     e.RecordType,
				Hostname: e.DNSName,
				Value:    e.Targets[0],
				TTL:      strconv.Itoa(ttl),
			},
		}

		changes = append(changes, change)
	}
	return changes
}

// separateChangesByZone organizes changes into zones for batch processing.
func separateChangesByZone(zones []utho.Domain, changes []*UthoChanges) map[string][]*UthoChanges {
	log.Infof("Separating changes by zone: zones=%v, changes=%v", zones, changes)
	change := make(map[string][]*UthoChanges)
	zoneNameID := provider.ZoneIDName{}

	// Build a mapping of zones for quick lookup.
	for _, z := range zones {
		log.Debugf("Adding zone: %s", z.Domain)
		zoneNameID.Add(z.Domain, z.Domain)
		change[z.Domain] = []*UthoChanges{}
	}

	// Match each change to its corresponding zone.
	for _, c := range changes {
		zone, _ := zoneNameID.FindZone(c.ResourceRecordSet.Hostname)
		if zone == "" {
			log.Debugf("Skipping record %s because no matching zone was detected", c.ResourceRecordSet.Hostname)
			continue
		}
		change[zone] = append(change[zone], c)
	}
	log.Infof("Zone-separated changes: %v", change)
	return change
}

// getRecordID retrieves the ID of a specific DNS record in a zone.
func (p *UthoProvider) getRecordID(zone string, record utho.CreateDnsRecordParams) (recordID string, err error) {
	log.Infof("Fetching record ID for zone: %s, record: %+v", zone, record)
	records, err := p.client.Domain().ListDnsRecords(zone)
	if err != nil {
		log.Errorf("Error fetching records for zone %s: %v", zone, err)
		return "0", err
	}

	// Find the record by matching its hostname and type.
	for _, r := range records {
		log.Debugf("Checking record: %+v", r)
		strippedName := strings.TrimSuffix(record.Hostname, "."+zone)
		if record.Hostname == zone {
			strippedName = ""
		}

		if r.Hostname == strippedName && r.Type == record.Type {
			log.Infof("Found matching record ID: %s", r.ID)
			return r.ID, nil
		}
	}

	log.Warnf("No record found for zone: %s, record: %+v", zone, record)
	return "", fmt.Errorf("no record was found")
}

// AdjustEndpoints ensures endpoints conform to the zone's requirements.
func (p *UthoProvider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	log.Infof("Adjusting endpoints: %v", endpoints)
	adjustedEndpoints := []*endpoint.Endpoint{}

	for _, ep := range endpoints {
		log.Debugf("Adjusting endpoint: %v", ep)
		_, zoneName := p.zoneIDNameMapper.FindZone(ep.DNSName)
		adjustedTargets := endpoint.Targets{}
		for _, t := range ep.Targets {
			// Normalize the target to conform to the zone's domain.
			var adjustedTarget, producedValidTarget = p.makeEndpointTarget(zoneName, t)
			if producedValidTarget {
				adjustedTargets = append(adjustedTargets, adjustedTarget)
			}
		}

		ep.Targets = adjustedTargets
		adjustedEndpoints = append(adjustedEndpoints, ep)
	}

	log.Infof("Adjusted endpoints: %v", adjustedEndpoints)
	return adjustedEndpoints, nil
}

// makeEndpointTarget normalizes a target to remove unnecessary suffixes.
func (p UthoProvider) makeEndpointTarget(domain, entryTarget string) (string, bool) {
	log.Debugf("Making endpoint target: domain=%s, target=%s", domain, entryTarget)
	if domain == "" {
		return entryTarget, true
	}

	adjustedTarget := strings.TrimSuffix(entryTarget, `.`)
	adjustedTarget = strings.TrimSuffix(adjustedTarget, "."+domain)

	return adjustedTarget, true
}

// GetDomainFilter constructs a domain filter based on the provider configuration.
func GetDomainFilter(config Configuration) endpoint.DomainFilter {
	log.Infof("Getting domain filter for config: %+v", config)
	var domainFilter endpoint.DomainFilter
	createMsg := "Creating Utho provider with "

	if config.RegexDomainFilter != "" {
		createMsg += fmt.Sprintf("Regexp domain filter: '%s', ", config.RegexDomainFilter)
		if config.RegexDomainExclusion != "" {
			createMsg += fmt.Sprintf("with exclusion: '%s', ", config.RegexDomainExclusion)
		}
		domainFilter = endpoint.NewRegexDomainFilter(
			regexp.MustCompile(config.RegexDomainFilter),
			regexp.MustCompile(config.RegexDomainExclusion),
		)
	} else {
		if len(config.DomainFilter) > 0 {
			createMsg += fmt.Sprintf("zoneNode filter: '%s', ", strings.Join(config.DomainFilter, ","))
		}
		if len(config.ExcludeDomains) > 0 {
			createMsg += fmt.Sprintf("Exclude domain filter: '%s', ", strings.Join(config.ExcludeDomains, ","))
		}
		domainFilter = endpoint.NewDomainFilterWithExclusions(config.DomainFilter, config.ExcludeDomains)
	}

	createMsg = strings.TrimSuffix(createMsg, ", ")
	if strings.HasSuffix(createMsg, "with ") {
		createMsg += "no kind of domain filters"
	}
	log.Info(createMsg)
	return domainFilter
}
