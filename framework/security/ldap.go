package security

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/NeftaliAcosta/springo/framework/config"

	"github.com/go-ldap/ldap/v3"
)

// LdapProperties holds the Active Directory / LDAP settings
type LdapProperties struct {
	Enabled           bool   `yaml:"enabled"`
	Urls              string `yaml:"urls"` // e.g. ldap://localhost:389, ldaps://localhost:636
	BaseDN            string `yaml:"base-dn"`
	UserSearchFilter  string `yaml:"user-search-filter"`  // e.g. (uid={0}) or (sAMAccountName={0})
	GroupSearchBase   string `yaml:"group-search-base"`   // e.g. ou=groups
	GroupSearchFilter string `yaml:"group-search-filter"` // e.g. (member={0})
	ManagerDN         string `yaml:"manager-dn"`
	ManagerPassword   string `yaml:"manager-password"`
	TLSSkipVerify     bool   `yaml:"tls-skip-verify"`
	StartTLS          bool   `yaml:"start-tls"`
	RequireTLS        bool   `yaml:"require-tls"`
}

func init() {
	config.RegisterProperties("spring.security.ldap", &LdapProperties{
		Enabled:           false,
		UserSearchFilter:  "(uid={0})",
		GroupSearchFilter: "(member={0})",
		StartTLS:          true,
		RequireTLS:        false,
	})
}

// LdapAuthenticationProvider handles user credential verification against LDAP/Active Directory
type LdapAuthenticationProvider struct {
	Properties *LdapProperties
}

// NewLdapAuthenticationProvider creates a new provider instance
func NewLdapAuthenticationProvider(props *LdapProperties) *LdapAuthenticationProvider {
	return &LdapAuthenticationProvider{Properties: props}
}

// Authenticate verifies the user's password and fetches group memberships
func (p *LdapAuthenticationProvider) Authenticate(username, password string) (string, []string, error) {
	rawUrl, scheme, err := p.validateInputs(username, password)
	if err != nil {
		return "", nil, err
	}

	conn, err := p.dialLDAP(rawUrl, scheme)
	if err != nil {
		return "", nil, fmt.Errorf("failed to connect to LDAP server: %w", err)
	}
	defer conn.Close()

	userDN, err := p.searchUser(conn, username)
	if err != nil {
		return "", nil, err
	}

	err = p.authenticateUser(conn, userDN, password)
	if err != nil {
		return "", nil, err
	}

	roles := p.searchGroups(conn, userDN)
	return username, roles, nil
}

func (p *LdapAuthenticationProvider) validateInputs(username, password string) (string, string, error) {
	if !p.Properties.Enabled {
		return "", "", fmt.Errorf("LDAP authentication is disabled")
	}
	if username == "" || password == "" {
		return "", "", fmt.Errorf("username and password cannot be empty")
	}

	rawUrl := strings.TrimSpace(p.Properties.Urls)
	if rawUrl == "" {
		return "", "", fmt.Errorf("LDAP URL is not configured")
	}

	u, err := url.Parse(rawUrl)
	if err != nil {
		return "", "", fmt.Errorf("invalid LDAP URL %q: %w", rawUrl, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "ldap" && scheme != "ldaps" {
		return "", "", fmt.Errorf("unsupported LDAP scheme %q, must be 'ldap' or 'ldaps'", u.Scheme)
	}
	return rawUrl, scheme, nil
}

func (p *LdapAuthenticationProvider) dialLDAP(rawUrl, scheme string) (*ldap.Conn, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: p.Properties.TLSSkipVerify}

	profile := os.Getenv("SPRINGO_PROFILES_ACTIVE")
	isProd := profile == "prod" || profile == "production"
	requireTLS := p.Properties.RequireTLS
	if isProd && scheme != "ldaps" {
		requireTLS = true
	}

	if scheme == "ldaps" {
		return ldap.DialURL(rawUrl, ldap.DialWithTLSConfig(tlsConfig))
	}

	conn, err := ldap.DialURL(rawUrl)
	if err != nil {
		return nil, err
	}

	if p.Properties.StartTLS {
		if startTLSErr := conn.StartTLS(tlsConfig); startTLSErr != nil {
			if requireTLS {
				conn.Close()
				return nil, fmt.Errorf("LDAP connection requires secure StartTLS, but TLS handshake failed: %w", startTLSErr)
			}
			log.Printf("⚠️ WARNING: LDAP StartTLS failed (%v). Falling back to INSECURE plaintext transmission on %s!", startTLSErr, rawUrl)
		}
	} else {
		if requireTLS {
			conn.Close()
			return nil, fmt.Errorf("LDAP connection requires secure TLS in production, but StartTLS is disabled and URL is not secure (ldaps://)")
		}
		log.Printf("⚠️ WARNING: LDAP connection is using INSECURE plaintext transmission on %s (StartTLS is disabled)!", rawUrl)
	}

	return conn, nil
}

func (p *LdapAuthenticationProvider) searchUser(conn *ldap.Conn, username string) (string, error) {
	if p.Properties.ManagerDN != "" {
		err := conn.Bind(p.Properties.ManagerDN, p.Properties.ManagerPassword)
		if err != nil {
			return "", fmt.Errorf("LDAP manager bind failed: %w", err)
		}
	}

	filter := strings.Replace(p.Properties.UserSearchFilter, "{0}", ldap.EscapeFilter(username), -1)
	searchRequest := ldap.NewSearchRequest(
		p.Properties.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filter,
		[]string{"dn", "cn"},
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return "", fmt.Errorf("LDAP user search error: %w", err)
	}

	if len(sr.Entries) == 0 {
		return "", fmt.Errorf("user not found in LDAP directory")
	}
	if len(sr.Entries) > 1 {
		return "", fmt.Errorf("multiple matching users found in LDAP")
	}

	return sr.Entries[0].DN, nil
}

func (p *LdapAuthenticationProvider) authenticateUser(conn *ldap.Conn, userDN, password string) error {
	err := conn.Bind(userDN, password)
	if err != nil {
		return fmt.Errorf("invalid LDAP credentials: %w", err)
	}
	return nil
}

func (p *LdapAuthenticationProvider) searchGroups(conn *ldap.Conn, userDN string) []string {
	if p.Properties.ManagerDN != "" {
		_ = conn.Bind(p.Properties.ManagerDN, p.Properties.ManagerPassword)
	}

	var roles []string
	groupSearchBase := p.Properties.GroupSearchBase
	if groupSearchBase == "" {
		groupSearchBase = p.Properties.BaseDN
	}

	escapedDN := ldap.EscapeFilter(userDN)
	groupFilter := strings.Replace(p.Properties.GroupSearchFilter, "{0}", escapedDN, -1)

	groupSearch := ldap.NewSearchRequest(
		groupSearchBase,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		groupFilter,
		[]string{"cn"},
		nil,
	)

	gs, err := conn.Search(groupSearch)
	if err == nil {
		for _, entry := range gs.Entries {
			cn := entry.GetAttributeValue("cn")
			if cn != "" {
				roleName := strings.ToUpper(cn)
				if !strings.HasPrefix(roleName, "ROLE_") {
					roleName = "ROLE_" + roleName
				}
				roles = append(roles, roleName)
			}
		}
	}

	return roles
}
