using System;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using Allure.Net.Commons;
using Xunit.v3;

namespace Sulu.XUnit;

/// <summary>
/// LOCAL drop-in replacement for the former <c>Sulu.XUnit.SuluTestAttribute</c> from
/// the deleted <c>sulu-dotnet-xunit</c> adapter. Kept under the original namespace +
/// name so all <c>[Fact, SuluTest("PETSTORE-NNN")]</c> call sites compile with ZERO edits.
///
/// xUnit v3 has no Allure binding (Allure.Xunit targets v2 + a VSTest reporter that
/// cannot hook the v3 console host), so this attribute writes the allure-results JSON
/// itself via <see cref="AllureLifecycle"/>. suluctl then uploads those files; Sulu's
/// AllureImportService reads testId from the <c>sulu_id</c> label.
///
/// <see cref="Before"/> records the start instant; <see cref="After"/> reads the outcome
/// from <c>Xunit.TestContext.Current.TestState</c> and emits one complete result.
/// The assembly runs serially (CollectionBehavior MaxParallelThreads=1) so the
/// AsyncLocal lifecycle context is collision-free.
/// </summary>
[AttributeUsage(AttributeTargets.Method, AllowMultiple = false)]
public sealed class SuluTestAttribute : BeforeAfterTestAttribute
{
    private static readonly AsyncLocal<DateTimeOffset?> _start = new();

    /// <summary>The Sulu TestCase id, e.g. "PETSTORE-006". Emitted as the sulu_id label.</summary>
    public string TestId { get; }

    public SuluTestAttribute(string testId) => TestId = testId;

    public override void Before(MethodInfo methodUnderTest, IXunitTest test)
    {
        _start.Value = DateTimeOffset.UtcNow;
    }

    public override void After(MethodInfo methodUnderTest, IXunitTest test)
    {
        var start = _start.Value ?? DateTimeOffset.UtcNow;
        _start.Value = null;
        var stop = DateTimeOffset.UtcNow;

        var state = Xunit.TestContext.Current?.TestState;
        var (status, statusDetails) = MapStatus(state);

        var displayName = string.IsNullOrWhiteSpace(test.TestDisplayName)
            ? methodUnderTest.Name
            : test.TestDisplayName;
        var fullName = methodUnderTest.DeclaringType is null
            ? methodUnderTest.Name
            : $"{methodUnderTest.DeclaringType.FullName}.{methodUnderTest.Name}";

        var uuid = Guid.NewGuid().ToString("N");
        var result = new TestResult
        {
            uuid = uuid,
            historyId = TestId,                       // stable retry-collapse key
            fullName = fullName,
            name = displayName,                       // -> Sulu launch display name
            status = status,
            statusDetails = statusDetails,
            start = start.ToUnixTimeMilliseconds(),
            stop = stop.ToUnixTimeMilliseconds(),
            labels = new System.Collections.Generic.List<Label>
            {
                new Label { name = "sulu_id", value = TestId },   // -> Sulu testId
            },
        };

        // Allure.Net.Commons 2.15.0 lifecycle: ScheduleTestCase(TestResult) activates the
        // context; Start/Stop/WriteTestCase are no-arg and operate on the current context.
        var lifecycle = AllureLifecycle.Instance;
        lifecycle.ScheduleTestCase(result);
        lifecycle.StartTestCase();
        lifecycle.StopTestCase();
        lifecycle.WriteTestCase();
    }

    private static (Status, StatusDetails?) MapStatus(Xunit.TestResultState? state)
    {
        if (state is null)
            return (Status.broken, null);

        var message = state.ExceptionMessages is { Length: > 0 } m ? m[0] : null;
        var trace = state.ExceptionStackTraces is { Length: > 0 } t ? t[0] : null;
        var details = (message is null && trace is null)
            ? null
            : new StatusDetails { message = message, trace = trace };

        return state.Result switch
        {
            Xunit.TestResult.Passed  => (Status.passed, null),
            Xunit.TestResult.Failed  => (Status.failed, details),
            Xunit.TestResult.Skipped => (Status.skipped, details),
            _                        => (Status.broken, details),
        };
    }

    // Kept for parity with the allure historyId fallback; not used (historyId is set to TestId).
    private static string Sha1(string value)
    {
        using var sha1 = SHA1.Create();
        var bytes = sha1.ComputeHash(Encoding.UTF8.GetBytes(value));
        var sb = new StringBuilder(bytes.Length * 2);
        foreach (var b in bytes) sb.Append(b.ToString("x2"));
        return sb.ToString();
    }
}
