package signature

import (
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/plugins"
	"github.com/grafana/grafana/pkg/setting"
)

var logger = log.New("plugin.signature.validator")

type Validator struct {
	cfg *setting.Cfg
	// allowUnsignedPluginsCondition changes the policy for allowing unsigned plugins. Signature validation only
	// runs when plugins are starting, therefore running plugins will not be terminated if they violate the new policy.
	allowUnsignedPluginsCondition UnsignedPluginConditionFunc
}

type UnsignedPluginConditionFunc = func(plugin *plugins.Plugin) bool

func NewValidator(cfg *setting.Cfg, unsignedCond UnsignedPluginConditionFunc) Validator {
	return Validator{
		cfg:                           cfg,
		allowUnsignedPluginsCondition: unsignedCond,
	}
}

func (s *Validator) Validate(plugin *plugins.Plugin) *plugins.PluginSignatureError {
	if plugin.Signature == plugins.SignatureValid {
		logger.Debug("Plugin has valid signature", "id", plugin.ID)
		return nil
	}

	// If a plugin is nested within another, create links to each other to inherit signature details
	if plugin.Parent != nil {
		if plugin.IsCorePlugin() || plugin.Signature == plugins.SignatureInternal {
			logger.Debug("Not setting descendant plugin's signature to that of root since it's core or internal",
				"plugin", plugin.ID, "signature", plugin.Signature, "isCore", plugin.IsCorePlugin)
		} else {
			logger.Debug("Setting descendant plugin's signature to that of root", "plugin", plugin.ID,
				"root", plugin.Parent.ID, "signature", plugin.Signature, "rootSignature", plugin.Parent.Signature)
			plugin.Signature = plugin.Parent.Signature
			plugin.SignatureType = plugin.Parent.SignatureType
			plugin.SignatureOrg = plugin.Parent.SignatureOrg
			if plugin.Signature == plugins.SignatureValid {
				logger.Debug("Plugin has valid signature (inherited from root)", "id", plugin.ID)
				return nil
			}
		}
	}

	if plugin.IsCorePlugin() || plugin.IsBundledPlugin() {
		return nil
	}

	switch plugin.Signature {
	case plugins.SignatureUnsigned:
		if allowed := s.allowUnsigned(plugin); !allowed {
			logger.Debug("Plugin is unsigned", "pluginID", plugin.ID)
			return &plugins.PluginSignatureError{
				PluginID:        plugin.ID,
				SignatureStatus: plugins.SignatureUnsigned,
			}
		}
		logger.Warn("Running an unsigned plugin", "pluginID", plugin.ID, "pluginDir", plugin.PluginDir)
		return nil
	case plugins.SignatureInvalid:
		logger.Debug("Plugin has an invalid signature", "pluginID", plugin.ID)
		return &plugins.PluginSignatureError{
			PluginID:        plugin.ID,
			SignatureStatus: plugins.SignatureInvalid,
		}
	case plugins.SignatureModified:
		logger.Debug("Plugin has a modified signature", "pluginID", plugin.ID)
		return &plugins.PluginSignatureError{
			PluginID:        plugin.ID,
			SignatureStatus: plugins.SignatureModified,
		}
	default:
		logger.Debug("Plugin has an unrecognized plugin signature state", "pluginID", plugin.ID, "signature",
			plugin.Signature)
		return &plugins.PluginSignatureError{
			PluginID: plugin.ID,
		}
	}
}

func (s *Validator) allowUnsigned(plugin *plugins.Plugin) bool {
	if s.allowUnsignedPluginsCondition != nil {
		return s.allowUnsignedPluginsCondition(plugin)
	}

	if s.cfg.Env == setting.Dev {
		return true
	}

	for _, plug := range s.cfg.PluginsAllowUnsigned {
		if plug == plugin.ID {
			return true
		}
	}

	return false
}
