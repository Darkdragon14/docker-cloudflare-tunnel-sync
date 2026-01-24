package access

import (
	"context"
	"sort"
	"strings"

	"log/slog"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/cloudflare"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/model"
)

// Engine reconciles Access applications and policies.
type Engine struct {
	api        cloudflare.AccessAPI
	log        *slog.Logger
	dryRun     bool
	manage     bool
	managedTag string
}

func NewEngine(api cloudflare.AccessAPI, logger *slog.Logger, dryRun bool, manage bool, managedBy string) *Engine {
	return &Engine{
		api:        api,
		log:        logger,
		dryRun:     dryRun,
		manage:     manage,
		managedTag: model.AccessManagedTag(managedBy),
	}
}

func (engine *Engine) Reconcile(ctx context.Context, apps []model.AccessAppSpec) error {
	if len(apps) == 0 && !engine.manage {
		return nil
	}

	existingApps, err := engine.api.ListAccessApps(ctx)
	if err != nil {
		return err
	}

	var existingPolicies []cloudflare.AccessPolicyRecord
	if len(apps) > 0 {
		existingPolicies, err = engine.api.ListAccessPolicies(ctx)
		if err != nil {
			return err
		}
	}

	appByID := map[string]cloudflare.AccessAppRecord{}
	appByKey := map[accessAppKey][]cloudflare.AccessAppRecord{}
	for _, app := range existingApps {
		if app.ID != "" {
			appByID[app.ID] = app
		}
		key := accessAppKey{Name: strings.ToLower(app.Name), Domain: strings.ToLower(app.Domain)}
		appByKey[key] = append(appByKey[key], app)
	}

	policyByID := map[string]cloudflare.AccessPolicyRecord{}
	policyByName := map[string][]cloudflare.AccessPolicyRecord{}
	for _, policy := range existingPolicies {
		if policy.ID != "" {
			policyByID[policy.ID] = policy
		}
		if policy.Name != "" {
			key := strings.ToLower(policy.Name)
			policyByName[key] = append(policyByName[key], policy)
		}
	}

	desiredAppIDs := map[string]struct{}{}
	for _, app := range apps {
		tagging := false
		if engine.manage {
			if err := engine.api.EnsureAccessTag(ctx, engine.managedTag); err != nil {
				engine.log.Warn("failed to ensure access tag; proceeding without tagging", "tag", engine.managedTag, "error", err)
			} else {
				tagging = true
			}
		}

		policyRefs, ok := engine.ensurePolicies(ctx, app, policyByID, policyByName)
		if !ok {
			continue
		}

		appSpec := app
		if engine.manage && app.TagsSet && len(app.Tags) > 0 {
			ensuredTags, tagsOK := engine.ensureAppTags(ctx, app)
			if !tagsOK {
				engine.log.Warn("access app tags could not be ensured; keeping existing tags", "app", app.Name)
				appSpec.TagsSet = false
			} else {
				appSpec.Tags = ensuredTags
			}
		}

		appRecord, found := engine.resolveAccessApp(appSpec, appByID, appByKey)
		if !found {
			if !engine.manage {
				engine.log.Warn("access app missing but SYNC_MANAGED_ACCESS is false; skipping create", "app", app.Name)
				continue
			}
			if engine.dryRun {
				engine.log.Info("would create access app", "app", app.Name)
				continue
			}
			created, err := engine.api.CreateAccessApp(ctx, engine.buildAppInput(appSpec, policyRefs, nil, tagging))
			if err != nil {
				engine.log.Error("failed to create access app", "app", app.Name, "error", err)
				continue
			}
			appByID[created.ID] = created
			desiredAppIDs[created.ID] = struct{}{}
			continue
		}

		desiredAppIDs[appRecord.ID] = struct{}{}
		input := engine.buildAppInput(appSpec, policyRefs, appRecord.Tags, tagging)
		if !engine.appNeedsUpdate(appRecord, input) {
			engine.log.Debug("access app up-to-date", "app", app.Name)
			continue
		}
		if !engine.manage {
			engine.log.Warn("access app differs but SYNC_MANAGED_ACCESS is false; skipping update", "app", app.Name)
			continue
		}
		engine.log.Info("updating access app", "app", app.Name)
		if engine.dryRun {
			continue
		}
		updated, err := engine.api.UpdateAccessApp(ctx, appRecord.ID, input)
		if err != nil {
			engine.log.Error("failed to update access app", "app", app.Name, "error", err)
			continue
		}
		appByID[updated.ID] = updated
	}

	engine.deleteOrphanedApps(ctx, existingApps, desiredAppIDs)
	return nil
}

func (engine *Engine) ensurePolicies(ctx context.Context, app model.AccessAppSpec, policyByID map[string]cloudflare.AccessPolicyRecord, policyByName map[string][]cloudflare.AccessPolicyRecord) ([]cloudflare.AccessPolicyRef, bool) {
	policyRefs := make([]cloudflare.AccessPolicyRef, 0, len(app.Policies))
	for _, policy := range app.Policies {
		precedence := len(policyRefs) + 1
		if policy.ID != "" {
			record, ok := policyByID[policy.ID]
			if !ok {
				if !policy.Managed {
					engine.log.Warn("access policy id not found in account policies; using id-only reference", "policy", policy.ID, "app", app.Name)
					policyRefs = append(policyRefs, cloudflare.AccessPolicyRef{ID: policy.ID, Precedence: precedence})
					continue
				}
				engine.log.Warn("access policy id not found", "policy", policyLabel(policy), "app", app.Name)
				return nil, false
			}
			policyRefs = append(policyRefs, cloudflare.AccessPolicyRef{ID: record.ID, Precedence: precedence})
			engine.updatePolicyIfNeeded(ctx, app, policy, record)
			continue
		}

		if !policy.Managed {
			record, found, ok := engine.resolvePolicyByName(policy, policyByName)
			if !ok {
				return nil, false
			}
			if !found {
				engine.log.Warn("access policy name not found; skipping access app", "policy", policyLabel(policy), "app", app.Name)
				return nil, false
			}
			policyRefs = append(policyRefs, cloudflare.AccessPolicyRef{ID: record.ID, Precedence: precedence})
			engine.updatePolicyIfNeeded(ctx, app, policy, record)
			continue
		}

		record, found, ok := engine.resolvePolicyByName(policy, policyByName)
		if !ok {
			return nil, false
		}
		if !found {
			if !engine.manage {
				engine.log.Warn("access policy missing but SYNC_MANAGED_ACCESS is false; skipping create", "policy", policyLabel(policy), "app", app.Name)
				continue
			}
			engine.log.Info("creating access policy", "policy", policyLabel(policy), "app", app.Name)
			if engine.dryRun {
				continue
			}
			created, err := engine.api.CreateAccessPolicy(ctx, engine.buildPolicyInput(policy))
			if err != nil {
				engine.log.Error("failed to create access policy", "policy", policyLabel(policy), "error", err)
				return nil, false
			}
			policyByID[created.ID] = created
			policyByName[strings.ToLower(created.Name)] = append(policyByName[strings.ToLower(created.Name)], created)
			policyRefs = append(policyRefs, cloudflare.AccessPolicyRef{ID: created.ID, Precedence: precedence})
			continue
		}

		policyRefs = append(policyRefs, cloudflare.AccessPolicyRef{ID: record.ID, Precedence: precedence})
		engine.updatePolicyIfNeeded(ctx, app, policy, record)
	}

	return policyRefs, len(policyRefs) > 0
}

func (engine *Engine) resolvePolicyByName(spec model.AccessPolicySpec, policyByName map[string][]cloudflare.AccessPolicyRecord) (cloudflare.AccessPolicyRecord, bool, bool) {
	matches := policyByName[strings.ToLower(spec.Name)]
	if len(matches) == 0 {
		return cloudflare.AccessPolicyRecord{}, false, true
	}
	if len(matches) > 1 {
		engine.log.Warn("multiple access policies share the same name; skipping", "policy", spec.Name)
		return cloudflare.AccessPolicyRecord{}, false, false
	}
	return matches[0], true, true
}

func (engine *Engine) updatePolicyIfNeeded(ctx context.Context, app model.AccessAppSpec, spec model.AccessPolicySpec, record cloudflare.AccessPolicyRecord) {
	if !spec.Managed {
		engine.log.Debug("access policy reference-only; skipping updates", "policy", policyLabel(spec))
		return
	}
	if record.HasUnsupportedRules {
		engine.log.Warn("access policy has unsupported rule types; rules will be replaced", "policy", policyLabel(spec))
	}
	if !policyNeedsUpdate(spec, record) {
		engine.log.Debug("access policy up-to-date", "policy", policyLabel(spec))
		return
	}
	if !engine.manage {
		engine.log.Warn("access policy differs but SYNC_MANAGED_ACCESS is false; skipping update", "policy", policyLabel(spec))
		return
	}
	engine.log.Info("updating access policy", "policy", policyLabel(spec), "app", app.Name)
	if engine.dryRun {
		return
	}
	_, err := engine.api.UpdateAccessPolicy(ctx, record.ID, engine.buildPolicyInput(spec))
	if err != nil {
		engine.log.Error("failed to update access policy", "policy", policyLabel(spec), "error", err)
		return
	}
}

func (engine *Engine) ensureAppTags(ctx context.Context, app model.AccessAppSpec) ([]string, bool) {
	if len(app.Tags) == 0 {
		return app.Tags, true
	}

	seen := map[string]struct{}{}
	ensured := make([]string, 0, len(app.Tags))
	ok := true
	for _, tag := range app.Tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		if err := engine.api.EnsureAccessTag(ctx, trimmed); err != nil {
			engine.log.Warn("failed to ensure access tag for app", "app", app.Name, "tag", trimmed, "error", err)
			ok = false
			continue
		}
		ensured = append(ensured, trimmed)
	}
	return ensured, ok
}

func (engine *Engine) resolveAccessApp(spec model.AccessAppSpec, appByID map[string]cloudflare.AccessAppRecord, appByKey map[accessAppKey][]cloudflare.AccessAppRecord) (cloudflare.AccessAppRecord, bool) {
	if spec.ID != "" {
		record, ok := appByID[spec.ID]
		if !ok {
			engine.log.Warn("access app id not found", "app", spec.Name, "id", spec.ID)
			return cloudflare.AccessAppRecord{}, false
		}
		return record, true
	}

	key := accessAppKey{Name: strings.ToLower(spec.Name), Domain: strings.ToLower(spec.Domain)}
	matches := appByKey[key]
	if len(matches) == 0 {
		return cloudflare.AccessAppRecord{}, false
	}
	if len(matches) > 1 {
		engine.log.Warn("multiple access apps share the same name and domain; skipping", "app", spec.Name)
		return cloudflare.AccessAppRecord{}, false
	}
	return matches[0], true
}

func (engine *Engine) buildPolicyInput(spec model.AccessPolicySpec) cloudflare.AccessPolicyInput {
	includes := make([]cloudflare.AccessRule, 0, len(spec.IncludeEmails)+len(spec.IncludeIPs))
	for _, email := range spec.IncludeEmails {
		includes = append(includes, cloudflare.AccessRule{Email: email})
	}
	for _, ip := range spec.IncludeIPs {
		includes = append(includes, cloudflare.AccessRule{IP: ip})
	}
	return cloudflare.AccessPolicyInput{
		Name:    spec.Name,
		Action:  spec.Action,
		Include: includes,
	}
}

func (engine *Engine) buildAppInput(spec model.AccessAppSpec, policyRefs []cloudflare.AccessPolicyRef, existingTags []string, tagging bool) cloudflare.AccessAppInput {
	tags := existingTags
	if spec.TagsSet {
		tags = spec.Tags
	}
	if tagging {
		tags = mergeTags(tags, engine.managedTag)
	}

	return cloudflare.AccessAppInput{
		Name:     spec.Name,
		Domain:   spec.Domain,
		Type:     "self_hosted",
		Policies: policyRefs,
		Tags:     tags,
	}
}

func (engine *Engine) appNeedsUpdate(record cloudflare.AccessAppRecord, desired cloudflare.AccessAppInput) bool {
	if record.Name != desired.Name {
		return true
	}
	if record.Domain != desired.Domain {
		return true
	}
	if record.Type != "" && record.Type != desired.Type {
		return true
	}
	if !policyRefsEqual(record.Policies, desired.Policies) {
		return true
	}
	if !stringSetsEqual(record.Tags, desired.Tags) {
		return true
	}
	return false
}

func (engine *Engine) deleteOrphanedApps(ctx context.Context, existing []cloudflare.AccessAppRecord, desired map[string]struct{}) {
	if !engine.manage {
		return
	}

	for _, app := range existing {
		if _, wanted := desired[app.ID]; wanted {
			continue
		}
		if !hasManagedTag(app.Tags, engine.managedTag) {
			continue
		}
		engine.log.Warn("managed access app no longer desired; deleting", "app", app.Name)
		if engine.dryRun {
			continue
		}
		if err := engine.api.DeleteAccessApp(ctx, app.ID); err != nil {
			engine.log.Error("failed to delete access app", "app", app.Name, "error", err)
		}
	}
}

type accessAppKey struct {
	Name   string
	Domain string
}

func policyNeedsUpdate(spec model.AccessPolicySpec, record cloudflare.AccessPolicyRecord) bool {
	if strings.ToLower(record.Action) != strings.ToLower(spec.Action) {
		return true
	}
	desired := normalizeRules(spec.IncludeEmails, spec.IncludeIPs)
	current := normalizeRuleList(record.Include)
	if len(desired) != len(current) {
		return true
	}
	for i := range desired {
		if desired[i] != current[i] {
			return true
		}
	}
	return false
}

func policyLabel(spec model.AccessPolicySpec) string {
	if spec.Name != "" {
		return spec.Name
	}
	if spec.ID != "" {
		return spec.ID
	}
	return "unknown"
}

func normalizeRules(emails []string, ips []string) []string {
	result := make([]string, 0, len(emails)+len(ips))
	for _, email := range emails {
		result = append(result, "email:"+strings.ToLower(strings.TrimSpace(email)))
	}
	for _, ip := range ips {
		result = append(result, "ip:"+strings.ToLower(strings.TrimSpace(ip)))
	}
	sort.Strings(result)
	return result
}

func normalizeRuleList(rules []cloudflare.AccessRule) []string {
	result := make([]string, 0, len(rules))
	for _, rule := range rules {
		if rule.Email != "" {
			result = append(result, "email:"+strings.ToLower(rule.Email))
		}
		if rule.IP != "" {
			result = append(result, "ip:"+strings.ToLower(rule.IP))
		}
	}
	sort.Strings(result)
	return result
}

func mergeTags(existing []string, required string) []string {
	tags := make([]string, 0, len(existing)+1)
	seen := map[string]struct{}{}
	for _, tag := range existing {
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	if required != "" {
		if _, ok := seen[required]; !ok {
			tags = append(tags, required)
		}
	}
	return tags
}

func hasManagedTag(tags []string, managedTag string) bool {
	for _, tag := range tags {
		if tag == managedTag {
			return true
		}
	}
	return false
}

func policyRefsEqual(left []cloudflare.AccessPolicyRef, right []cloudflare.AccessPolicyRef) bool {
	if len(left) != len(right) {
		return false
	}
	leftKeys := normalizePolicyRefs(left)
	rightKeys := normalizePolicyRefs(right)
	for i := range leftKeys {
		if leftKeys[i] != rightKeys[i] {
			return false
		}
	}
	return true
}

func normalizePolicyRefs(refs []cloudflare.AccessPolicyRef) []string {
	ordered := make([]struct {
		ID    string
		Order int
	}, 0, len(refs))
	for index, ref := range refs {
		if ref.ID == "" {
			continue
		}
		order := ref.Precedence
		if order == 0 {
			order = index + 1
		}
		ordered = append(ordered, struct {
			ID    string
			Order int
		}{ID: ref.ID, Order: order})
	}
	if len(ordered) == 0 {
		return nil
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Order < ordered[j].Order
	})
	result := make([]string, 0, len(ordered))
	for _, item := range ordered {
		result = append(result, item.ID)
	}
	return result
}

func stringSetsEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]string{}, left...)
	rightCopy := append([]string{}, right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for i := range leftCopy {
		if leftCopy[i] != rightCopy[i] {
			return false
		}
	}
	return true
}
