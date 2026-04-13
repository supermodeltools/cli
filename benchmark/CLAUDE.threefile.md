# Django Source — supermodel three-file shards enabled

This is the Django framework source. The auth package is at `django/contrib/auth/`.

## Graph shard files

`supermodel analyze --three-file` has run on this repo. Every source file has
three shard files with pre-computed context:

- `.calls.py` — function call relationships (who calls what, with file and line number)
- `.deps.py` — import dependencies (what this file imports and what imports it)
- `.impact.py` — blast radius (risk level, affected domains, direct/transitive dependents)

**Read the shard files before the source file.** They show you the full
picture in far fewer tokens. For example:

- Wondering what `django/contrib/auth/__init__.py` calls and who calls it?
  → read `django/contrib/auth/__init__.calls.py`
- Need to know what this module depends on?
  → read `django/contrib/auth/__init__.deps.py`
- Want to assess blast radius before changing something?
  → read `django/contrib/auth/__init__.impact.py`

Use the shard files to navigate efficiently. Only drop into the source when you
need implementation details the shards don't cover.
