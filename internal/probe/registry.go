package probe

func DefaultRegistry(client *BaseRequest, enableCodeWikiRPC bool) map[Source]Probe {
	return map[Source]Probe{
		SourceDeepWiki: NewDeepWikiProbe(client),
		SourceZread:    NewZreadProbe(client),
		SourceCodeWiki: NewCodeWikiProbe(client, enableCodeWikiRPC),
	}
}
