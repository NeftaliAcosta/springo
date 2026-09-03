# 🏢 Step-by-Step Guide: Corporate Security & Active Directory LDAP

This tutorial explains how to configure enterprise LDAP and Microsoft Active Directory authentication in SprinGo.

---

## 1. Overview

SprinGo includes native LDAP/Active Directory support:
- **Direct & Search Bind**: Supports anonymous binds, service account searches, and direct user binds.
- **TLS / StartTLS Encryption**: Secure credential transmission with custom CA certificate authorities.
- **Group-to-Role Mapping**: Automatically maps LDAP organizational groups (e.g. `CN=Admins,OU=Groups`) to app roles.
- **Failover Redundancy**: Supports secondary and tertiary backup LDAP server hosts.

---

## 2. Configuration

**Suggested File Path**: `resources/application.yaml`
```yaml
spring:
  security:
    ldap:
      enabled: true
      host: ${LDAP_HOST:ldap.company.internal}
      port: 636
      use-ssl: true
      base-dn: "dc=company,dc=internal"
      user-dn-pattern: "uid={0},ou=users,dc=company,dc=internal"
      bind-dn: "cn=admin,dc=company,dc=internal"
      bind-password: ${LDAP_BIND_PASSWORD}
      user-search-base: "ou=users"
      user-search-filter: "(&(objectClass=person)(mail={0}))"
      group-search-base: "ou=groups"
      group-search-filter: "(member={0})"
      group-role-attribute: "cn"
```

---

## 3. Authenticating Users via LDAP Provider

**Suggested File Path**: `internal/application/service/auth_service.go`
```go
package service

import (
    "context"
    "fmt"

    "github.com/NeftaliAcosta/springo/framework/security"
)

type AuthService struct {
    ldapProvider *security.LDAPProvider `spring:"ldapProvider"`
}

func (s *AuthService) Login(
    ctx context.Context,
    username string,
    password string,
) (*security.UserIdentity, error) {
    user, err := s.ldapProvider.Authenticate(ctx, username, password)
    if err != nil {
        return nil, fmt.Errorf("ldap authentication failed: %w", err)
    }

    // user.Roles contains groups mapped to application roles
    return user, nil
}
```
