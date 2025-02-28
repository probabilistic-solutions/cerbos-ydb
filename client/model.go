// Copyright 2021-2023 Zenauth Ltd.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/multierr"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	auditv1 "github.com/cerbos/cerbos/api/genpb/cerbos/audit/v1"
	effectv1 "github.com/cerbos/cerbos/api/genpb/cerbos/effect/v1"
	enginev1 "github.com/cerbos/cerbos/api/genpb/cerbos/engine/v1"
	policyv1 "github.com/cerbos/cerbos/api/genpb/cerbos/policy/v1"
	requestv1 "github.com/cerbos/cerbos/api/genpb/cerbos/request/v1"
	responsev1 "github.com/cerbos/cerbos/api/genpb/cerbos/response/v1"
	schemav1 "github.com/cerbos/cerbos/api/genpb/cerbos/schema/v1"
	"github.com/cerbos/cerbos/internal/policy"
	"github.com/cerbos/cerbos/internal/schema"
	"github.com/cerbos/cerbos/internal/util"
)

const apiVersion = "api.cerbos.dev/v1"

// Principal is a container for principal data.
type Principal struct {
	p   *enginev1.Principal
	err error
}

// NewPrincipal creates a new principal object with the given ID and roles.
func NewPrincipal(id string, roles ...string) *Principal {
	return &Principal{
		p: &enginev1.Principal{
			Id:    id,
			Roles: roles,
		},
	}
}

// WithPolicyVersion sets the policy version for this principal.
func (p *Principal) WithPolicyVersion(policyVersion string) *Principal {
	p.p.PolicyVersion = policyVersion
	return p
}

// WithRoles appends the set of roles to principal's existing roles.
func (p *Principal) WithRoles(roles ...string) *Principal {
	p.p.Roles = append(p.p.Roles, roles...)
	return p
}

// WithScope sets the scope this principal belongs to.
func (p *Principal) WithScope(scope string) *Principal {
	p.p.Scope = scope
	return p
}

// WithAttributes merges the given attributes to principal's existing attributes.
func (p *Principal) WithAttributes(attr map[string]any) *Principal {
	if p.p.Attr == nil {
		p.p.Attr = make(map[string]*structpb.Value, len(attr))
	}

	for k, v := range attr {
		pbVal, err := util.ToStructPB(v)
		if err != nil {
			p.err = multierr.Append(p.err, fmt.Errorf("invalid attribute value for '%s': %w", k, err))
			continue
		}
		p.p.Attr[k] = pbVal
	}

	return p
}

// WithAttr adds a new attribute to the principal.
// It will overwrite any existing attribute having the same key.
func (p *Principal) WithAttr(key string, value any) *Principal {
	if p.p.Attr == nil {
		p.p.Attr = make(map[string]*structpb.Value)
	}

	pbVal, err := util.ToStructPB(value)
	if err != nil {
		p.err = multierr.Append(p.err, fmt.Errorf("invalid attribute value for '%s': %w", key, err))
		return p
	}

	p.p.Attr[key] = pbVal
	return p
}

// ID returns the principal ID.
func (p *Principal) ID() string {
	return p.p.GetId()
}

// Roles returns the principal roles.
func (p *Principal) Roles() []string {
	return p.p.GetRoles()
}

// Proto returns the underlying protobuf object representing the principal.
func (p *Principal) Proto() *enginev1.Principal {
	return p.p
}

// Err returns any errors accumulated during the construction of the principal.
func (p *Principal) Err() error {
	return p.err
}

// Validate checks whether the principal object is valid.
func (p *Principal) Validate() error {
	if p.err != nil {
		return p.err
	}

	return p.p.Validate()
}

// Resource is a single resource instance.
type Resource struct {
	r   *enginev1.Resource
	err error
}

// NewResource creates a new instance of a resource.
func NewResource(kind, id string) *Resource {
	return &Resource{
		r: &enginev1.Resource{Kind: kind, Id: id},
	}
}

// WithPolicyVersion sets the policy version for this resource.
func (r *Resource) WithPolicyVersion(policyVersion string) *Resource {
	r.r.PolicyVersion = policyVersion
	return r
}

// WithAttributes merges the given attributes to the resource's existing attributes.
func (r *Resource) WithAttributes(attr map[string]any) *Resource {
	if r.r.Attr == nil {
		r.r.Attr = make(map[string]*structpb.Value, len(attr))
	}

	for k, v := range attr {
		pbVal, err := util.ToStructPB(v)
		if err != nil {
			r.err = multierr.Append(r.err, fmt.Errorf("invalid attribute value for '%s': %w", k, err))
			continue
		}
		r.r.Attr[k] = pbVal
	}

	return r
}

// WithAttr adds a new attribute to the resource.
// It will overwrite any existing attribute having the same key.
func (r *Resource) WithAttr(key string, value any) *Resource {
	if r.r.Attr == nil {
		r.r.Attr = make(map[string]*structpb.Value)
	}

	pbVal, err := util.ToStructPB(value)
	if err != nil {
		r.err = multierr.Append(r.err, fmt.Errorf("invalid attribute value for '%s': %w", key, err))
		return r
	}

	r.r.Attr[key] = pbVal
	return r
}

// WithScope sets the scope this resource belongs to.
func (r *Resource) WithScope(scope string) *Resource {
	r.r.Scope = scope
	return r
}

// ID returns the resource ID.
func (r *Resource) ID() string {
	return r.r.GetId()
}

// Kind returns the resource kind.
func (r *Resource) Kind() string {
	return r.r.GetKind()
}

// Proto returns the underlying protobuf object representing the resource.
func (r *Resource) Proto() *enginev1.Resource {
	return r.r
}

// Err returns any errors accumulated during the construction of the resource.
func (r *Resource) Err() error {
	return r.err
}

// Validate checks whether the resource is valid.
func (r *Resource) Validate() error {
	if r.err != nil {
		return r.err
	}

	return r.r.Validate()
}

// ResourceSet is a container for a set of resources of the same kind.
type ResourceSet struct {
	rs  *requestv1.ResourceSet
	err error
}

// NewResourceSet creates a new resource set.
func NewResourceSet(kind string) *ResourceSet {
	return &ResourceSet{
		rs: &requestv1.ResourceSet{Kind: kind},
	}
}

// WithPolicyVersion sets the policy version for this resource set.
func (rs *ResourceSet) WithPolicyVersion(policyVersion string) *ResourceSet {
	rs.rs.PolicyVersion = policyVersion
	return rs
}

// AddResourceInstance adds a new resource instance to the resource set.
func (rs *ResourceSet) AddResourceInstance(id string, attr map[string]any) *ResourceSet {
	if rs.rs.Instances == nil {
		rs.rs.Instances = make(map[string]*requestv1.AttributesMap)
	}

	pbAttr := make(map[string]*structpb.Value, len(attr))
	for k, v := range attr {
		pbVal, err := structpb.NewValue(v)
		if err != nil {
			rs.err = multierr.Append(rs.err, fmt.Errorf("invalid attribute value for '%s': %w", k, err))
			continue
		}
		pbAttr[k] = pbVal
	}

	rs.rs.Instances[id] = &requestv1.AttributesMap{Attr: pbAttr}
	return rs
}

// Err returns any errors accumulated during the construction of this resource set.
func (rs *ResourceSet) Err() error {
	return rs.err
}

// Validate checks whether this resource set is valid.
func (rs *ResourceSet) Validate() error {
	if rs.err != nil {
		return rs.err
	}

	return rs.rs.Validate()
}

// CheckResourceSetResponse is the response from the CheckResourceSet API call.
type CheckResourceSetResponse struct {
	*responsev1.CheckResourceSetResponse
}

// IsAllowed returns true if the response indicates that the given action on the given resource is allowed.
// If the resource or action is not contained in the response, the return value will always be false.
func (crsr *CheckResourceSetResponse) IsAllowed(resourceID, action string) bool {
	res, ok := crsr.ResourceInstances[resourceID]
	if !ok {
		return false
	}

	effect, ok := res.Actions[action]
	if !ok {
		return false
	}

	return effect == effectv1.Effect_EFFECT_ALLOW
}

// Errors returns all validation errors returned by the server.
func (crsr *CheckResourceSetResponse) Errors() error {
	var err error
	for resource, actions := range crsr.ResourceInstances {
		for _, verr := range actions.ValidationErrors {
			err = multierr.Append(err,
				fmt.Errorf("resource %q failed validation: source=%s path=%s msg=%s", resource, verr.Source, verr.Path, verr.Message),
			)
		}
	}

	return err
}

func (crsr *CheckResourceSetResponse) String() string {
	return protojson.Format(crsr.CheckResourceSetResponse)
}

func (crsr *CheckResourceSetResponse) MarshalJSON() ([]byte, error) {
	return protojson.Marshal(crsr.CheckResourceSetResponse)
}

// ResourceBatch is a container for a batch of heterogeneous resources.
type ResourceBatch struct {
	err   error
	batch []*requestv1.CheckResourcesRequest_ResourceEntry
}

// NewResourceBatch creates a new resource batch.
func NewResourceBatch() *ResourceBatch {
	return &ResourceBatch{}
}

// Add a new resource to the batch.
func (rb *ResourceBatch) Add(resource *Resource, actions ...string) *ResourceBatch {
	if resource == nil || len(actions) == 0 {
		return rb
	}

	entry := &requestv1.CheckResourcesRequest_ResourceEntry{
		Actions:  actions,
		Resource: resource.r,
	}

	if err := entry.Validate(); err != nil {
		rb.err = multierr.Append(rb.err, fmt.Errorf("invalid resource '%s': %w", resource.r.Id, err))
		return rb
	}

	rb.batch = append(rb.batch, entry)
	return rb
}

// Err returns any errors accumulated during the construction of the resource batch.
func (rb *ResourceBatch) Err() error {
	return rb.err
}

// Validate checks whether the resource batch is valid.
func (rb *ResourceBatch) Validate() error {
	if rb.err != nil {
		return rb.err
	}

	if len(rb.batch) == 0 {
		return errors.New("empty batch")
	}

	var errList error
	for _, entry := range rb.batch {
		if err := entry.Validate(); err != nil {
			errList = multierr.Append(errList, err)
		}
	}

	return errList
}

func (rb *ResourceBatch) toResourceBatchEntry() []*requestv1.CheckResourceBatchRequest_BatchEntry {
	b := make([]*requestv1.CheckResourceBatchRequest_BatchEntry, len(rb.batch))
	for i, r := range rb.batch {
		b[i] = &requestv1.CheckResourceBatchRequest_BatchEntry{
			Resource: r.Resource,
			Actions:  r.Actions,
		}
	}

	return b
}

// CheckResourceBatchResponse is the response from the CheckResourceBatch API call.
type CheckResourceBatchResponse struct {
	*responsev1.CheckResourceBatchResponse
	idx  map[string][]int
	once sync.Once
}

func (crbr *CheckResourceBatchResponse) buildIdx() {
	crbr.once.Do(func() {
		crbr.idx = make(map[string][]int, len(crbr.Results))
		for i, r := range crbr.Results {
			v := crbr.idx[r.ResourceId]
			crbr.idx[r.ResourceId] = append(v, i)
		}
	})
}

// IsAllowed returns true if the given resource and action is allowed.
// If the resource or the action is not included in the response, the result will always be false.
func (crbr *CheckResourceBatchResponse) IsAllowed(resourceID, action string) bool {
	crbr.buildIdx()
	indexes, ok := crbr.idx[resourceID]
	if !ok {
		return false
	}

	for _, i := range indexes {
		r := crbr.Results[i]
		if r == nil {
			continue
		}

		if effect, ok := r.Actions[action]; ok {
			return effect == effectv1.Effect_EFFECT_ALLOW
		}
	}

	return false
}

// Errors returns any validation errors returned by the server.
func (crbr *CheckResourceBatchResponse) Errors() error {
	var err error
	for _, result := range crbr.Results {
		for _, verr := range result.ValidationErrors {
			err = multierr.Append(err,
				fmt.Errorf("resource %q failed validation: source=%s path=%s msg=%s", result.ResourceId, verr.Source, verr.Path, verr.Message),
			)
		}
	}

	return err
}

func (crbr *CheckResourceBatchResponse) String() string {
	return protojson.Format(crbr.CheckResourceBatchResponse)
}

func (crbr *CheckResourceBatchResponse) MarshalJSON() ([]byte, error) {
	return protojson.Marshal(crbr.CheckResourceBatchResponse)
}

type ResourceResult struct {
	*responsev1.CheckResourcesResponse_ResultEntry
	err        error
	outputMap  map[string]*structpb.Value
	outputOnce sync.Once
}

func (rr *ResourceResult) Err() error {
	return rr.err
}

// IsAllowed returns true if the given action is allowed.
// Returns false if the action is not in the response of if there was an error getting this result.
func (rr *ResourceResult) IsAllowed(action string) bool {
	if rr != nil && rr.err == nil {
		return rr.Actions[action] == effectv1.Effect_EFFECT_ALLOW
	}

	return false
}

func (rr *ResourceResult) buildOutputMap() {
	rr.outputOnce.Do(func() {
		if len(rr.GetOutputs()) == 0 {
			return
		}

		rr.outputMap = make(map[string]*structpb.Value, len(rr.Outputs))
		for _, o := range rr.Outputs {
			rr.outputMap[o.GetSrc()] = o.GetVal()
		}
	})
}

func (rr *ResourceResult) Output(key string) *structpb.Value {
	if rr == nil {
		return nil
	}

	rr.buildOutputMap()
	return rr.outputMap[key]
}

// MatchResource is a function that returns true if the given resource is of interest.
// This is useful when you have more than one resource with the same ID and need to distinguish
// between them in the response.
type MatchResource func(*responsev1.CheckResourcesResponse_ResultEntry_Resource) bool

// MatchResourceKind is a matcher that checks that the resource kind matches the given value.
func MatchResourceKind(kind string) MatchResource {
	return func(r *responsev1.CheckResourcesResponse_ResultEntry_Resource) bool {
		return r.Kind == kind
	}
}

// MatchResourceScope is a matcher that checks that the resource scope matches the given value.
func MatchResourceScope(scope string) MatchResource {
	return func(r *responsev1.CheckResourcesResponse_ResultEntry_Resource) bool {
		return r.Scope == scope
	}
}

// MatchResourcePolicyVersion is a matcher that checks that the resource policy version matches the given value.
func MatchResourcePolicyVersion(version string) MatchResource {
	return func(r *responsev1.CheckResourcesResponse_ResultEntry_Resource) bool {
		return r.PolicyVersion == version
	}
}

// MatchResourcePolicyKindScopeVersion is a matcher that checks that the resource policy kind, version and scope matches the given values.
func MatchResourcePolicyKindScopeVersion(kind, version, scope string) MatchResource {
	return func(r *responsev1.CheckResourcesResponse_ResultEntry_Resource) bool {
		return r.Kind == kind && r.PolicyVersion == version && r.Scope == scope
	}
}

// CheckResourcesResponse is the response from the CheckResources API call.
type CheckResourcesResponse struct {
	*responsev1.CheckResourcesResponse
	idx  map[string][]int
	once sync.Once
}

func (crr *CheckResourcesResponse) buildIdx() {
	crr.once.Do(func() {
		crr.idx = make(map[string][]int, len(crr.Results))
		for i, r := range crr.Results {
			v := crr.idx[r.Resource.Id]
			crr.idx[r.Resource.Id] = append(v, i)
		}
	})
}

// GetResource finds the resource with the given ID and optional properties from the result list.
// Returns a ResourceResult object with the Err field set if the resource is not found.
func (crr *CheckResourcesResponse) GetResource(resourceID string, match ...MatchResource) *ResourceResult {
	crr.buildIdx()

	indexes, ok := crr.idx[resourceID]
	if !ok {
		return &ResourceResult{err: fmt.Errorf("resource with ID %q does not exist in the response", resourceID)}
	}

	for _, i := range indexes {
		r := crr.Results[i]
		if r == nil {
			continue
		}

		found := true
		for _, m := range match {
			found = found && m(r.Resource)
		}

		if found {
			return &ResourceResult{CheckResourcesResponse_ResultEntry: r}
		}
	}

	return &ResourceResult{err: fmt.Errorf("resource with ID %q does not exist in the response", resourceID)}
}

// Errors returns any validation errors returned by the server.
func (crr *CheckResourcesResponse) Errors() error {
	var err error
	for _, result := range crr.Results {
		for _, verr := range result.ValidationErrors {
			err = multierr.Append(err,
				fmt.Errorf("resource %q failed validation: source=%s path=%s msg=%s", result.Resource.Id, verr.Source, verr.Path, verr.Message),
			)
		}
	}

	return err
}

func (crr *CheckResourcesResponse) String() string {
	return protojson.Format(crr.CheckResourcesResponse)
}

func (crr *CheckResourcesResponse) MarshalJSON() ([]byte, error) {
	return protojson.Marshal(crr.CheckResourcesResponse)
}

// PolicySet is a container for a set of policies.
type PolicySet struct {
	err      error
	policies []*policyv1.Policy
}

// NewPolicySet creates a new policy set.
func NewPolicySet() *PolicySet {
	return &PolicySet{}
}

// AddPolicyFromFile adds a policy from the given file to the set.
func (ps *PolicySet) AddPolicyFromFile(file string) *PolicySet {
	f, err := os.Open(file)
	if err != nil {
		ps.err = multierr.Append(ps.err, fmt.Errorf("failed to add policy from file '%s': %w", file, err))
		return ps
	}

	defer f.Close()
	return ps.AddPolicyFromReader(f)
}

// AddPolicyFromFileWithErr adds a policy from the given file to the set and returns the error.
func (ps *PolicySet) AddPolicyFromFileWithErr(file string) (*PolicySet, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", file, err)
	}
	defer f.Close()

	p, err := policy.ReadPolicy(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy: %w", err)
	}

	return ps.AddPolicies(p), nil
}

// AddPolicyFromReader adds a policy from the given reader to the set.
func (ps *PolicySet) AddPolicyFromReader(r io.Reader) *PolicySet {
	p, err := policy.ReadPolicy(r)
	if err != nil {
		ps.err = multierr.Append(ps.err, fmt.Errorf("failed to add policy from reader: %w", err))
		return ps
	}

	ps.policies = append(ps.policies, p)
	return ps
}

// AddPolicies adds the given policies to the set.
func (ps *PolicySet) AddPolicies(policies ...*policyv1.Policy) *PolicySet {
	ps.policies = append(ps.policies, policies...)
	return ps
}

// AddResourcePolicies adds the given resource policies to the set.
func (ps *PolicySet) AddResourcePolicies(policies ...*ResourcePolicy) *PolicySet {
	for _, p := range policies {
		if p == nil {
			continue
		}

		if err := ps.add(p); err != nil {
			ps.err = multierr.Append(ps.err, fmt.Errorf("failed to add resource policy [%s:%s]: %w", p.p.Resource, p.p.Version, err))
		}
	}

	return ps
}

// AddPrincipalPolicies adds the given principal policies to the set.
func (ps *PolicySet) AddPrincipalPolicies(policies ...*PrincipalPolicy) *PolicySet {
	for _, p := range policies {
		if p == nil {
			continue
		}

		if err := ps.add(p); err != nil {
			ps.err = multierr.Append(ps.err, fmt.Errorf("failed to add principal policy [%s:%s]: %w", p.pp.Principal, p.pp.Version, err))
		}
	}

	return ps
}

// AddDerivedRoles adds the given derived roles to the set.
func (ps *PolicySet) AddDerivedRoles(policies ...*DerivedRoles) *PolicySet {
	for _, p := range policies {
		if p == nil {
			continue
		}

		if err := ps.add(p); err != nil {
			ps.err = multierr.Append(ps.err, fmt.Errorf("failed to add derived roles [%s]: %w", p.dr.Name, err))
		}
	}

	return ps
}

// AddExportVariables adds the given exported variables to the set.
func (ps *PolicySet) AddExportVariables(policies ...*ExportVariables) *PolicySet {
	for _, p := range policies {
		if p == nil {
			continue
		}

		if err := ps.add(p); err != nil {
			ps.err = multierr.Append(ps.err, fmt.Errorf("failed to add exported variables [%s]: %w", p.ev.Name, err))
		}
	}

	return ps
}

// GetPolicies returns all of the policies in the set.
func (ps *PolicySet) GetPolicies() []*policyv1.Policy {
	return ps.policies
}

// Size returns the number of policies in this set.
func (ps *PolicySet) Size() int {
	return len(ps.policies)
}

func (ps *PolicySet) add(b interface {
	build() (*policyv1.Policy, error)
},
) error {
	p, err := b.build()
	if err != nil {
		return err
	}

	ps.policies = append(ps.policies, p)
	return nil
}

// Err returns the errors accumulated during the construction of the policy set.
func (ps *PolicySet) Err() error {
	return ps.err
}

// Validate checks whether the policy set is valid.
func (ps *PolicySet) Validate() error {
	if ps.err != nil {
		return ps.err
	}

	if len(ps.policies) == 0 {
		return errors.New("empty policy set")
	}

	return nil
}

// SchemaSet is a container for a set of schemas.
type SchemaSet struct {
	err     error
	schemas []*schemav1.Schema
}

// NewSchemaSet creates a new schema set.
func NewSchemaSet() *SchemaSet {
	return &SchemaSet{}
}

// AddSchemaFromFile adds a schema from the given file to the set.
func (ss *SchemaSet) AddSchemaFromFile(file string, ignorePathInID bool) *SchemaSet {
	f, err := os.Open(file)
	if err != nil {
		ss.err = multierr.Append(ss.err, fmt.Errorf("failed to add schema from file '%s': %w", file, err))
		return ss
	}

	name := file
	if ignorePathInID {
		name = filepath.Base(name)
	}

	defer f.Close()
	return ss.AddSchemaFromReader(f, name)
}

// AddSchemaFromFileWithErr adds a schema from the given file to the set and returns the error.
func (ss *SchemaSet) AddSchemaFromFileWithErr(file string, ignorePathInID bool) (*SchemaSet, error) {
	id := file
	if ignorePathInID {
		id = filepath.Base(id)
	}

	return ss.AddSchemaFromFileWithIDAndErr(file, id)
}

// AddSchemaFromFileWithIDAndErr adds a schema with the given id from the given file to the set and returns the error.
func (ss *SchemaSet) AddSchemaFromFileWithIDAndErr(file, id string) (*SchemaSet, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", file, err)
	}
	defer f.Close()

	s, err := schema.ReadSchema(f, id)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema: %w", err)
	}

	return ss.AddSchemas(s), nil
}

// AddSchemaFromReader adds a schema from the given reader to the set.
func (ss *SchemaSet) AddSchemaFromReader(r io.Reader, id string) *SchemaSet {
	s, err := schema.ReadSchema(r, id)
	if err != nil {
		ss.err = multierr.Append(ss.err, fmt.Errorf("failed to add schema from reader: %w", err))
		return ss
	}
	ss.schemas = append(ss.schemas, s)

	return ss
}

// AddSchemas adds the given schemas to the set.
func (ss *SchemaSet) AddSchemas(schemas ...*schemav1.Schema) *SchemaSet {
	ss.schemas = append(ss.schemas, schemas...)
	return ss
}

// GetSchemas returns all of the schemas in the set.
func (ss *SchemaSet) GetSchemas() []*schemav1.Schema {
	return ss.schemas
}

// Size returns the number of schemas in this set.
func (ss *SchemaSet) Size() int {
	return len(ss.schemas)
}

// Err returns the errors accumulated during the construction of the schema set.
func (ss *SchemaSet) Err() error {
	return ss.err
}

// Schema is a builder for Schemas_Schema.
type Schema struct {
	s *policyv1.Schemas_Schema
}

func NewSchema(ref string) *Schema {
	return (&Schema{
		s: &policyv1.Schemas_Schema{
			Ref:        "",
			IgnoreWhen: &policyv1.Schemas_IgnoreWhen{},
		},
	}).WithRef(ref)
}

// WithRef sets the ref of this schema.
func (s *Schema) WithRef(ref string) *Schema {
	s.s.Ref = ref
	return s
}

// AddIgnoredActions adds action(s) to the ignoreWhen field of the schema.
func (s *Schema) AddIgnoredActions(actions ...string) *Schema {
	s.s.IgnoreWhen.Actions = append(s.s.IgnoreWhen.Actions, actions...)
	return s
}

func (s *Schema) Validate() error {
	return s.s.Validate()
}

func (s *Schema) build() *policyv1.Schemas_Schema {
	return s.s
}

// ResourcePolicy is a builder for resource policies.
type ResourcePolicy struct {
	p   *policyv1.ResourcePolicy
	err error
}

// NewResourcePolicy creates a new resource policy builder.
func NewResourcePolicy(resource, version string) *ResourcePolicy {
	return &ResourcePolicy{
		p: &policyv1.ResourcePolicy{
			Resource:  resource,
			Version:   version,
			Variables: &policyv1.Variables{Local: make(map[string]string)},
		},
	}
}

// WithDerivedRolesImports adds import statements for derived roles.
func (rp *ResourcePolicy) WithDerivedRolesImports(imp ...string) *ResourcePolicy {
	rp.p.ImportDerivedRoles = append(rp.p.ImportDerivedRoles, imp...)
	return rp
}

func (rp *ResourcePolicy) WithScope(scope string) *ResourcePolicy {
	rp.p.Scope = scope
	return rp
}

func (rp *ResourcePolicy) WithPrincipalSchema(principalSchema *Schema) *ResourcePolicy {
	rp.p.Schemas.PrincipalSchema = principalSchema.build()
	return rp
}

func (rp *ResourcePolicy) WithResourceSchema(resourceSchema *Schema) *ResourcePolicy {
	rp.p.Schemas.ResourceSchema = resourceSchema.build()
	return rp
}

// AddResourceRules adds resource rules to the policy.
func (rp *ResourcePolicy) AddResourceRules(rules ...*ResourceRule) *ResourcePolicy {
	for _, r := range rules {
		if r == nil {
			continue
		}

		if err := r.Validate(); err != nil {
			rp.err = multierr.Append(rp.err, fmt.Errorf("invalid rule: %w", err))
			continue
		}

		rp.p.Rules = append(rp.p.Rules, r.rule)
	}

	return rp
}

// WithVariablesImports adds import statements for exported variables.
func (rp *ResourcePolicy) WithVariablesImports(name ...string) *ResourcePolicy {
	rp.p.Variables.Import = append(rp.p.Variables.Import, name...)
	return rp
}

// WithVariable adds a variable definition for use in conditions.
func (rp *ResourcePolicy) WithVariable(name, expr string) *ResourcePolicy {
	rp.p.Variables.Local[name] = expr
	return rp
}

// Err returns any errors accumulated during the construction of the policy.
func (rp *ResourcePolicy) Err() error {
	return rp.err
}

// Validate checks whether the policy is valid.
func (rp *ResourcePolicy) Validate() error {
	if rp.err != nil {
		return rp.err
	}

	_, err := rp.build()
	return err
}

func (rp *ResourcePolicy) build() (*policyv1.Policy, error) {
	p := &policyv1.Policy{
		ApiVersion: apiVersion,
		PolicyType: &policyv1.Policy_ResourcePolicy{
			ResourcePolicy: rp.p,
		},
	}

	return p, policy.Validate(p)
}

// ResourceRule is a rule in a resource policy.
type ResourceRule struct {
	rule *policyv1.ResourceRule
}

// NewAllowResourceRule creates a resource rule that allows the actions when matched.
func NewAllowResourceRule(actions ...string) *ResourceRule {
	return &ResourceRule{
		rule: &policyv1.ResourceRule{
			Actions: actions,
			Effect:  effectv1.Effect_EFFECT_ALLOW,
		},
	}
}

// NewDenyResourceRule creates a resource rule that denies the actions when matched.
func NewDenyResourceRule(actions ...string) *ResourceRule {
	return &ResourceRule{
		rule: &policyv1.ResourceRule{
			Actions: actions,
			Effect:  effectv1.Effect_EFFECT_DENY,
		},
	}
}

// WithName sets the name of the ResourceRule.
func (rr *ResourceRule) WithName(name string) *ResourceRule {
	rr.rule.Name = name
	return rr
}

// WithRoles adds roles to which this rule applies.
func (rr *ResourceRule) WithRoles(roles ...string) *ResourceRule {
	rr.rule.Roles = append(rr.rule.Roles, roles...)
	return rr
}

// WithDerivedRoles adds derived roles to which this rule applies.
func (rr *ResourceRule) WithDerivedRoles(roles ...string) *ResourceRule {
	rr.rule.DerivedRoles = append(rr.rule.DerivedRoles, roles...)
	return rr
}

// WithCondition sets the condition that applies to this rule.
func (rr *ResourceRule) WithCondition(m match) *ResourceRule {
	rr.rule.Condition = &policyv1.Condition{
		Condition: &policyv1.Condition_Match{
			Match: m.build(),
		},
	}

	return rr
}

// Err returns errors accumulated during the construction of the resource rule.
func (rr *ResourceRule) Err() error {
	return nil
}

// Validate checks whether the resource rule is valid.
func (rr *ResourceRule) Validate() error {
	return rr.rule.Validate()
}

// PrincipalPolicy is a builder for principal policies.
type PrincipalPolicy struct {
	pp  *policyv1.PrincipalPolicy
	err error
}

// NewPrincipalPolicy creates a new principal policy.
func NewPrincipalPolicy(principal, version string) *PrincipalPolicy {
	return &PrincipalPolicy{
		pp: &policyv1.PrincipalPolicy{
			Principal: principal,
			Version:   version,
			Variables: &policyv1.Variables{Local: make(map[string]string)},
		},
	}
}

// AddPrincipalRules adds rules to this policy.
func (pp *PrincipalPolicy) AddPrincipalRules(rules ...*PrincipalRule) *PrincipalPolicy {
	for _, r := range rules {
		if r == nil {
			continue
		}

		if err := r.Validate(); err != nil {
			pp.err = multierr.Append(pp.err, fmt.Errorf("invalid rule: %w", err))
			continue
		}

		pp.pp.Rules = append(pp.pp.Rules, r.rule)
	}

	return pp
}

// WithScope sets the scope of this policy.
func (pp *PrincipalPolicy) WithScope(scope string) *PrincipalPolicy {
	pp.pp.Scope = scope
	return pp
}

// WithVersion sets the version of this policy.
func (pp *PrincipalPolicy) WithVersion(version string) *PrincipalPolicy {
	pp.pp.Version = version
	return pp
}

// WithVariablesImports adds import statements for exported variables.
func (pp *PrincipalPolicy) WithVariablesImports(name ...string) *PrincipalPolicy {
	pp.pp.Variables.Import = append(pp.pp.Variables.Import, name...)
	return pp
}

// WithVariable adds a variable definition for use in conditions.
func (pp *PrincipalPolicy) WithVariable(name, expr string) *PrincipalPolicy {
	pp.pp.Variables.Local[name] = expr
	return pp
}

// Err returns the errors accumulated during the construction of this policy.
func (pp *PrincipalPolicy) Err() error {
	return pp.err
}

// Validate checks whether the policy is valid.
func (pp *PrincipalPolicy) Validate() error {
	if pp.err != nil {
		return pp.err
	}

	_, err := pp.build()
	return err
}

func (pp *PrincipalPolicy) build() (*policyv1.Policy, error) {
	p := &policyv1.Policy{
		ApiVersion: apiVersion,
		PolicyType: &policyv1.Policy_PrincipalPolicy{
			PrincipalPolicy: pp.pp,
		},
	}

	return p, policy.Validate(p)
}

// PrincipalRule is a builder for principal rules.
type PrincipalRule struct {
	rule *policyv1.PrincipalRule
}

// NewPrincipalRule creates a new rule for the specified resource.
func NewPrincipalRule(resource string) *PrincipalRule {
	return &PrincipalRule{
		rule: &policyv1.PrincipalRule{
			Resource: resource,
		},
	}
}

// AllowAction sets the action as allowed on the resource.
func (pr *PrincipalRule) AllowAction(action string) *PrincipalRule {
	return pr.addAction(action, effectv1.Effect_EFFECT_ALLOW, nil)
}

// DenyAction sets the action as denied on the resource.
func (pr *PrincipalRule) DenyAction(action string) *PrincipalRule {
	return pr.addAction(action, effectv1.Effect_EFFECT_DENY, nil)
}

// AllowActionOnCondition sets the action as allowed if the condition is fulfilled.
func (pr *PrincipalRule) AllowActionOnCondition(action string, m match) *PrincipalRule {
	cond := &policyv1.Condition{Condition: &policyv1.Condition_Match{Match: m.build()}}
	return pr.addAction(action, effectv1.Effect_EFFECT_ALLOW, cond)
}

// DenyActionOnCondition sets the action as denied if the condition is fulfilled.
func (pr *PrincipalRule) DenyActionOnCondition(action string, m match) *PrincipalRule {
	cond := &policyv1.Condition{Condition: &policyv1.Condition_Match{Match: m.build()}}
	return pr.addAction(action, effectv1.Effect_EFFECT_DENY, cond)
}

func (pr *PrincipalRule) addAction(action string, effect effectv1.Effect, comp *policyv1.Condition) *PrincipalRule {
	pr.rule.Actions = append(pr.rule.Actions, &policyv1.PrincipalRule_Action{
		Action:    action,
		Effect:    effect,
		Condition: comp,
	})

	return pr
}

// Err returns errors accumulated during the construction of the rule.
func (pr *PrincipalRule) Err() error {
	return nil
}

// Validate checks whether the rule is valid.
func (pr *PrincipalRule) Validate() error {
	return pr.rule.Validate()
}

// DerivedRoles is a builder for derived roles.
type DerivedRoles struct {
	dr *policyv1.DerivedRoles
}

// NewDerivedRoles creates a new derived roles set with the given name.
func NewDerivedRoles(name string) *DerivedRoles {
	return &DerivedRoles{
		dr: &policyv1.DerivedRoles{
			Name:      name,
			Variables: &policyv1.Variables{Local: make(map[string]string)},
		},
	}
}

// AddRole adds a new derived role with the given name which is an alias for the set of parent roles.
func (dr *DerivedRoles) AddRole(name string, parentRoles []string) *DerivedRoles {
	return dr.addRoleDef(name, parentRoles, nil)
}

// AddRoleWithCondition adds a derived role with a condition attached.
func (dr *DerivedRoles) AddRoleWithCondition(name string, parentRoles []string, m match) *DerivedRoles {
	cond := &policyv1.Condition{Condition: &policyv1.Condition_Match{Match: m.build()}}
	return dr.addRoleDef(name, parentRoles, cond)
}

func (dr *DerivedRoles) addRoleDef(name string, parentRoles []string, comp *policyv1.Condition) *DerivedRoles {
	dr.dr.Definitions = append(dr.dr.Definitions, &policyv1.RoleDef{Name: name, ParentRoles: parentRoles, Condition: comp})
	return dr
}

// WithVariablesImports adds import statements for exported variables.
func (dr *DerivedRoles) WithVariablesImports(name ...string) *DerivedRoles {
	dr.dr.Variables.Import = append(dr.dr.Variables.Import, name...)
	return dr
}

// WithVariable adds a variable definition for use in conditions.
func (dr *DerivedRoles) WithVariable(name, expr string) *DerivedRoles {
	dr.dr.Variables.Local[name] = expr
	return dr
}

// Err returns any errors accumulated during the construction of the derived roles.
func (dr *DerivedRoles) Err() error {
	return nil
}

// Validate checks whether the derived roles are valid.
func (dr *DerivedRoles) Validate() error {
	_, err := dr.build()
	return err
}

func (dr *DerivedRoles) build() (*policyv1.Policy, error) {
	p := &policyv1.Policy{
		ApiVersion: apiVersion,
		PolicyType: &policyv1.Policy_DerivedRoles{
			DerivedRoles: dr.dr,
		},
	}

	return p, policy.Validate(p)
}

// ExportVariables is a builder for exported variables.
type ExportVariables struct {
	ev *policyv1.ExportVariables
}

// NewExportVariables creates a new exported variables set with the given name.
func NewExportVariables(name string) *ExportVariables {
	return &ExportVariables{
		ev: &policyv1.ExportVariables{
			Name:        name,
			Definitions: make(map[string]string),
		},
	}
}

// AddVariable defines an exported variable with the given name to be computed by the given expression.
func (ev *ExportVariables) AddVariable(name, expr string) *ExportVariables {
	ev.ev.Definitions[name] = expr
	return ev
}

// Err returns any errors accumulated during the construction of the exported variables.
func (ev *ExportVariables) Err() error {
	return nil
}

// Validate checks whether the exported variables are valid.
func (ev *ExportVariables) Validate() error {
	_, err := ev.build()
	return err
}

func (ev *ExportVariables) build() (*policyv1.Policy, error) {
	p := &policyv1.Policy{
		ApiVersion: apiVersion,
		PolicyType: &policyv1.Policy_ExportVariables{
			ExportVariables: ev.ev,
		},
	}

	return p, policy.Validate(p)
}

// MatchExpr matches a single expression.
func MatchExpr(expr string) match {
	return matchExpr(expr)
}

// MatchAllOf matches all of the expressions (logical AND).
func MatchAllOf(m ...match) match {
	return matchList{
		list: m,
		cons: func(exprList []*policyv1.Match) *policyv1.Match {
			return &policyv1.Match{Op: &policyv1.Match_All{All: &policyv1.Match_ExprList{Of: exprList}}}
		},
	}
}

// MatchAnyOf  matches any of the expressions (logical OR).
func MatchAnyOf(m ...match) match {
	return matchList{
		list: m,
		cons: func(exprList []*policyv1.Match) *policyv1.Match {
			return &policyv1.Match{Op: &policyv1.Match_Any{Any: &policyv1.Match_ExprList{Of: exprList}}}
		},
	}
}

// MatchNoneOf  matches none of the expressions (logical NOT).
func MatchNoneOf(m ...match) match {
	return matchList{
		list: m,
		cons: func(exprList []*policyv1.Match) *policyv1.Match {
			return &policyv1.Match{Op: &policyv1.Match_None{None: &policyv1.Match_ExprList{Of: exprList}}}
		},
	}
}

type match interface {
	build() *policyv1.Match
}

type matchExpr string

func (me matchExpr) build() *policyv1.Match {
	expr := string(me)
	return &policyv1.Match{Op: &policyv1.Match_Expr{Expr: expr}}
}

type matchList struct {
	cons func([]*policyv1.Match) *policyv1.Match
	list []match
}

func (ml matchList) build() *policyv1.Match {
	exprList := make([]*policyv1.Match, len(ml.list))
	for i, expr := range ml.list {
		exprList[i] = expr.build()
	}

	return ml.cons(exprList)
}

type ServerInfo struct {
	*responsev1.ServerInfoResponse
}

func (si *ServerInfo) String() string {
	return protojson.Format(si.ServerInfoResponse)
}

func (si *ServerInfo) MarshalJSON() ([]byte, error) {
	return protojson.Marshal(si.ServerInfoResponse)
}

type AuditLogType uint8

const (
	AccessLogs AuditLogType = iota
	DecisionLogs
)

// AuditLogOptions is used to filter audit logs.
type AuditLogOptions struct {
	StartTime time.Time
	EndTime   time.Time
	Lookup    string
	Tail      uint32
	Type      AuditLogType
}

type AuditLogEntry struct {
	accessLog   *auditv1.AccessLogEntry
	decisionLog *auditv1.DecisionLogEntry
	err         error
}

func (e *AuditLogEntry) AccessLog() (*auditv1.AccessLogEntry, error) {
	return e.accessLog, e.err
}

func (e *AuditLogEntry) DecisionLog() (*auditv1.DecisionLogEntry, error) {
	return e.decisionLog, e.err
}

type PlanResourcesResponse struct {
	*responsev1.PlanResourcesResponse
}

type (
	ListPoliciesOption func(*requestv1.ListPoliciesRequest)
)

func WithIncludeDisabled() ListPoliciesOption {
	return func(request *requestv1.ListPoliciesRequest) {
		request.IncludeDisabled = true
	}
}

func WithNameRegexp(re string) ListPoliciesOption {
	return func(request *requestv1.ListPoliciesRequest) {
		request.NameRegexp = re
	}
}

func WithScopeRegexp(re string) ListPoliciesOption {
	return func(request *requestv1.ListPoliciesRequest) {
		request.ScopeRegexp = re
	}
}

func WithVersionRegexp(v string) ListPoliciesOption {
	return func(request *requestv1.ListPoliciesRequest) {
		request.VersionRegexp = v
	}
}
