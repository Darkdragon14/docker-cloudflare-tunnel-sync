package reconcile

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"

	"log/slog"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/cloudflare"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/model"
)

// Engine reconciles desired routes against the tunnel configuration.
type Engine struct {
	api          cloudflare.API
	log          *slog.Logger
	dryRun       bool
	manageTunnel bool
}

func NewEngine(api cloudflare.API, logger *slog.Logger, dryRun bool, manageTunnel bool) *Engine {
	return &Engine{api: api, log: logger, dryRun: dryRun, manageTunnel: manageTunnel}
}

func (engine *Engine) Reconcile(ctx context.Context, desired []model.RouteSpec) error {
	config, err := engine.api.GetConfig(ctx)
	if err != nil {
		return err
	}

	existingIngress := config.Ingress
	desiredIngress, removedRules := engine.buildDesiredIngress(desired, existingIngress)
	ingressMatches := ingressEqual(existingIngress, desiredIngress)

	for _, rule := range removedRules {
		engine.log.Warn("existing ingress rule not defined by labels; will be removed", "rule", ingressRuleKey(rule))
	}

	if ingressMatches {
		engine.log.Debug("tunnel ingress up-to-date", "rules", len(desiredIngress))
		return nil
	}

	if !engine.manageTunnel {
		engine.log.Warn("tunnel ingress differs but SYNC_MANAGED_TUNNEL is false; skipping update", "desired_rules", len(desiredIngress), "existing_rules", len(existingIngress))
		return nil
	}

	engine.log.Info("updating tunnel ingress", "desired_rules", len(desiredIngress), "existing_rules", len(existingIngress))
	if engine.dryRun {
		return nil
	}

	config.Ingress = desiredIngress
	return engine.api.UpdateConfig(ctx, config)
}

func (engine *Engine) buildDesiredIngress(desired []model.RouteSpec, existing []cloudflare.IngressRule) ([]cloudflare.IngressRule, []cloudflare.IngressRule) {
	existingByKey := map[model.RouteKey]cloudflare.IngressRule{}
	duplicates := map[model.RouteKey]struct{}{}
	for _, rule := range existing {
		if rule.Hostname == "" && rule.Service == model.FallbackService {
			continue
		}
		if rule.Hostname == "" {
			engine.log.Warn("existing ingress rule missing hostname; will be replaced", "service", rule.Service)
			continue
		}
		key := model.RouteKey{Hostname: rule.Hostname, Path: rule.Path}
		if _, exists := existingByKey[key]; exists {
			duplicates[key] = struct{}{}
			continue
		}
		existingByKey[key] = rule
	}

	for key := range duplicates {
		engine.log.Warn("duplicate ingress rules detected; keeping first", "rule", key.String())
	}

	desiredRules := make([]cloudflare.IngressRule, 0, len(desired)+1)
	desiredKeys := make(map[model.RouteKey]struct{}, len(desired))
	for _, route := range desired {
		var existingOriginRequest json.RawMessage
		if existingRule, ok := existingByKey[route.Key]; ok {
			existingOriginRequest = existingRule.OriginRequest
		}

		rule := cloudflare.IngressRule{
			Hostname:      route.Key.Hostname,
			Path:          route.Key.Path,
			Service:       route.Service,
			OriginRequest: mergeManagedOriginRequest(existingOriginRequest, route, engine.log),
		}
		desiredRules = append(desiredRules, rule)
		desiredKeys[route.Key] = struct{}{}
	}

	sort.Slice(desiredRules, func(i, j int) bool {
		return ingressRuleKey(desiredRules[i]) < ingressRuleKey(desiredRules[j])
	})

	removed := make([]cloudflare.IngressRule, 0)
	for key, rule := range existingByKey {
		if _, wanted := desiredKeys[key]; !wanted {
			removed = append(removed, rule)
		}
	}
	sort.Slice(removed, func(i, j int) bool {
		return ingressRuleKey(removed[i]) < ingressRuleKey(removed[j])
	})

	desiredRules = append(desiredRules, cloudflare.IngressRule{Service: model.FallbackService})

	return desiredRules, removed
}

func ingressEqual(left []cloudflare.IngressRule, right []cloudflare.IngressRule) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Hostname != right[i].Hostname {
			return false
		}
		if left[i].Path != right[i].Path {
			return false
		}
		if left[i].Service != right[i].Service {
			return false
		}
		if !bytes.Equal(left[i].OriginRequest, right[i].OriginRequest) {
			return false
		}
	}
	return true
}

func ingressRuleKey(rule cloudflare.IngressRule) string {
	return model.RouteKey{Hostname: rule.Hostname, Path: rule.Path}.String()
}

func mergeManagedOriginRequest(existing json.RawMessage, route model.RouteSpec, logger *slog.Logger) json.RawMessage {
	if len(existing) == 0 && route.OriginServerName == nil && route.NoTLSVerify == nil {
		return nil
	}

	originRequest := map[string]any{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &originRequest); err != nil {
			logger.Warn("existing originRequest is invalid JSON; rebuilding managed keys", "route", route.Key.String(), "error", err)
			originRequest = map[string]any{}
		}
	}

	changed := false
	if route.OriginServerName != nil {
		if current, ok := originRequest["originServerName"]; !ok || !originRequestStringEqual(current, *route.OriginServerName) {
			originRequest["originServerName"] = *route.OriginServerName
			changed = true
		}
	} else {
		if _, ok := originRequest["originServerName"]; ok {
			delete(originRequest, "originServerName")
			changed = true
		}
	}

	if route.NoTLSVerify != nil {
		if current, ok := originRequest["noTLSVerify"]; !ok || !originRequestBoolEqual(current, *route.NoTLSVerify) {
			originRequest["noTLSVerify"] = *route.NoTLSVerify
			changed = true
		}
	} else {
		if _, ok := originRequest["noTLSVerify"]; ok {
			delete(originRequest, "noTLSVerify")
			changed = true
		}
	}

	if !changed {
		if len(existing) == 0 {
			return nil
		}
		return existing
	}

	if len(originRequest) == 0 {
		return nil
	}

	merged, err := json.Marshal(originRequest)
	if err != nil {
		logger.Warn("failed to marshal managed originRequest", "route", route.Key.String(), "error", err)
		if len(existing) == 0 {
			return nil
		}
		return existing
	}

	return merged
}

func originRequestStringEqual(value any, expected string) bool {
	stringValue, ok := value.(string)
	return ok && stringValue == expected
}

func originRequestBoolEqual(value any, expected bool) bool {
	boolValue, ok := value.(bool)
	return ok && boolValue == expected
}
