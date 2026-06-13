"""Local drop-in replacement for the removed ``sulu-pytest`` adapter.

This module lives in the suite root (on pytest's ``sys.path``) and is imported
as ``sulu_pytest`` by the test files exactly as before — so no test body or
import line changes. Instead of POSTing results to Sulu's API at runtime, it
attaches the identity that Sulu's allure-results importer reads:

  * label ``sulu_id`` = the stable ``PETSTORE-NNN`` id  -> Sulu testId
  * label(s) ``tag``  = optional tags                   -> Sulu tags
  * the allure display name (when an explicit ``name`` is given)

allure-pytest (registered via its own pytest entry point) writes the
``allure-results/*-result.json`` files; ``suluctl watch`` uploads them.
"""
from __future__ import annotations

from typing import Callable, Sequence, TypeVar

import allure

__all__ = ["sulu_test", "step"]

# Existing call sites use ``with step("..."):`` — allure.step is a
# context-manager / decorator, a direct functional match.
step = allure.step

F = TypeVar("F", bound=Callable[..., object])


def sulu_test(*, id: str, name: str | None = None, tags: Sequence[str] = ()) -> Callable[[F], F]:
    """Decorator that stamps a test function with Sulu identity via static allure labels.

    ``id`` is required (keyword-only) and becomes the ``sulu_id`` allure label,
    which Sulu's AllureImportService resolves to the testId (PETSTORE-NNN).
    ``tags`` (if any) become ``tag`` allure labels; ``name`` (if given) sets the
    allure display name. Static decorators only — never dynamic + autouse, which
    would double the tags.
    """
    def decorator(func: F) -> F:
        wrapped: Callable[..., object] = func
        if name is not None:
            wrapped = allure.title(name)(wrapped)
        if tags:
            wrapped = allure.tag(*tags)(wrapped)
        wrapped = allure.label("sulu_id", id)(wrapped)
        return wrapped  # type: ignore[return-value]

    return decorator
