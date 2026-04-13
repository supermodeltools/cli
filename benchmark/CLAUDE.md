# Django Source — supermodel context enabled

This is the Django framework source. The auth package is at `django/contrib/auth/`.

## Graph shard files

`supermodel analyze` has run on this repo. Every source file has a `.graph.py`
sidecar with pre-computed context: imports, exports, callers, callees, and
cross-module relationships.

**Read the `.graph.py` file before the source file.** It shows you the full
picture in far fewer tokens. For example:

- Wondering what `django/contrib/auth/__init__.py` exports and what it calls?
  → read `django/contrib/auth/__init__.graph.py` first
- Need to know what signals the auth system emits and where they're defined?
  → check `django/contrib/auth/signals.graph.py`
- Want to see what depends on a given module?
  → the `.graph.py` file lists known callers

Use the graph files to navigate efficiently. Only drop into the source when you
need implementation details the graph doesn't cover.
