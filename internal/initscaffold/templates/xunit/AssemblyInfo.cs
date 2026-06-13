using Xunit;

// Serialize the whole assembly: Allure.Net.Commons keeps its current-test-case
// context in an AsyncLocal on a shared singleton (AllureLifecycle.Instance). Running
// tests in parallel would let concurrent After() hooks clobber each other's context,
// producing missing/duplicated *-result.json.
[assembly: CollectionBehavior(MaxParallelThreads = 1)]
