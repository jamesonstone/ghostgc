package main

import (
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func renderCacheArtifacts(response api.CacheArtifactsResponse) {
	if len(response.Artifacts) == 0 {
		fmt.Println("No cache artifacts matched.")
		fmt.Println(response.Note)
		return
	}
	for _, artifact := range response.Artifacts {
		fmt.Printf("%s  %-15s  %8d bytes  session %s  %s\n",
			artifact.ID, artifact.Lifecycle, artifact.Identity.Size, artifact.SessionID, artifact.RelativePath)
	}
	fmt.Println("\n" + response.Note)
}

func renderCacheArtifact(response api.CacheArtifactResponse) {
	artifact := response.Artifact
	fmt.Printf("Artifact: %s\nProvider: %s\nAgent: %s\nSession: %s\nKind: %s\nLifecycle: %s\nRoot: %s\nRelative path: %s\n",
		artifact.ID, artifact.Provider, artifact.Agent, artifact.SessionID, artifact.Kind,
		artifact.Lifecycle, artifact.RootPath, artifact.RelativePath)
	fmt.Printf("Identity: device %d inode %d uid %d links %d size %d\nManifest: %s\nReason: %s\n",
		artifact.Identity.Device, artifact.Identity.Inode, artifact.Identity.UID,
		artifact.Identity.Nlink, artifact.Identity.Size, artifact.ManifestDigest, artifact.Reason)
	if len(artifact.Evidence) > 0 {
		fmt.Println("Evidence:")
		for _, evidence := range artifact.Evidence {
			fmt.Println("  - " + evidence)
		}
	}
	fmt.Println(response.Note)
}

func renderCacheQuarantines(response api.CacheQuarantinesResponse) {
	if len(response.Artifacts) == 0 {
		fmt.Println("No cache artifacts are quarantined.")
		fmt.Println(response.Note)
		return
	}
	for _, item := range response.Artifacts {
		fmt.Printf("%s  %8d bytes  quarantined %s  purge eligible %s\n",
			item.ArtifactID, item.Identity.Size, formatTime(item.QuarantinedNs), formatTime(item.GraceUntilNs))
	}
	fmt.Println("\n" + response.Note)
}

func renderCacheActions(response api.CacheActionsResponse) {
	if len(response.Actions) == 0 {
		fmt.Println("No cache actions recorded.")
		return
	}
	for _, action := range response.Actions {
		fmt.Printf("%s  %-10s  %-11s  %s  %s\n", action.ActionID, action.Kind, action.Result, action.ArtifactID, action.Reason)
	}
}

func renderCachePreview(response api.CachePreviewResponse) {
	fmt.Printf("Cache %s preview for %s\nDestination: %s\nApproval expires: %s\n\nApply exactly this approval:\n  %s\n",
		response.Action, response.Artifact.ID, response.Destination, formatTime(response.ExpiresNs), response.Command)
	fmt.Println("\n" + response.Note)
}

func renderCacheResult(response api.CacheApplyResponse) {
	fmt.Printf("Cache action: %s\nArtifact: %s\nResult: %s\nAt: %s\nReason: %s\n",
		response.ActionID, response.ArtifactID, response.Result, formatTime(response.AtNs), response.Reason)
}

func formatTime(ns int64) string {
	if ns == 0 {
		return "unknown"
	}
	return time.Unix(0, ns).Format(time.RFC3339)
}
