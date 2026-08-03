package daemon

import (
	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/protection"
)

func (d *Daemon) protectionFor(p process.Process, tree *process.Tree, confidence float64, isRoot, sessionActive bool) protection.Result {
	var adapterRules []adapters.ProtectionRule
	for _, adapter := range d.reg.All() {
		adapterRules = append(adapterRules, adapter.ProtectedPatterns()...)
	}
	return protection.Evaluate(protection.Input{
		Process: p, SelfPID: d.selfPI, SelfUID: d.plat.SelfUID(), IsAgentRoot: isRoot,
		SessionActive: sessionActive, AttributionConfidence: confidence,
		DescendantCount: len(tree.Descendants(p.PID)), AdapterRules: adapterRules,
	})
}
