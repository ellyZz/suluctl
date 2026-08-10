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

// LogbackSetupSteps are the printed manual steps for registering the scaffolded
// SuluLog appender — emitted only when logback is detected and the glue was written.
func LogbackSetupSteps(pkg string) []string {
	if pkg == "" {
		pkg = "<your glue package>"
	}
	return []string{
		"Log capture: ensure logback-classic is a test dependency (e.g. testImplementation 'ch.qos.logback:logback-classic:1.5.12').",
		"Register the SuluLog appender in src/test/resources/logback-test.xml\n" +
			"    (logback resolves appenders by class name — there is no log4j2-style packages=\"…\" attribute):\n" +
			"    <appender name=\"SuluLog\" class=\"" + pkg + ".SuluLogAppender\">\n" +
			"      <encoder><pattern>%d{HH:mm:ss.SSS} %-5level %logger{1} - %msg%n</pattern></encoder>\n" +
			"    </appender>\n" +
			"    then add <appender-ref ref=\"SuluLog\"/> inside <root>.",
	}
}

// LogHintSteps tell a Java project where NEITHER logging framework was detected how
// to opt into per-test logs. Both options are named; the appender is scaffolded for
// whichever the build file references (log4j2 wins if both are).
func LogHintSteps() []string {
	return []string{
		"Per-test logs: no logging framework found in the build file — the SuluLog appender was skipped.\n" +
			"    Add log4j-core (log4j2) OR logback-classic as a test dependency, then re-run `suluctl init --force`.\n" +
			"    Note: logback often arrives transitively (e.g. via spring-boot-starter-test), which this\n" +
			"    build-file check cannot see — declare it explicitly to opt in.",
	}
}
