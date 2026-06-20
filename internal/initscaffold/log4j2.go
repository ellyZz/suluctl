package initscaffold

// Log4j2SetupSteps are the printed manual steps for registering the scaffolded
// SuluLog appender — emitted only when log4j2 is detected and the glue was written.
func Log4j2SetupSteps(pkg string) []string {
	if pkg == "" {
		pkg = "<your glue package>"
	}
	return []string{
		"Log capture: ensure log4j-core is a test dependency (e.g. testImplementation 'org.apache.logging.log4j:log4j-core:2.23.1').",
		"Register the SuluLog appender in src/test/resources/log4j2.xml:\n" +
			"    set <Configuration packages=\"" + pkg + "\"> so log4j2 resolves the custom element,\n" +
			"    add <SuluLog name=\"SuluLog\"><PatternLayout pattern=\"%d{HH:mm:ss.SSS} %-5level %logger{1} - %msg%n\"/></SuluLog>,\n" +
			"    then add <AppenderRef ref=\"SuluLog\"/> inside <Root>.",
	}
}

// Log4j2HintSteps tell a Java project WITHOUT log4j2 how to opt into per-test logs.
func Log4j2HintSteps() []string {
	return []string{
		"Per-test logs: add log4j-core (log4j2) as a test dependency and re-run `suluctl init --force` to scaffold the SuluLog appender.",
	}
}
