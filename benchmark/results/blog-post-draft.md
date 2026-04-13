# 60% cheaper. 4× faster. Same correct answer.

We ran a test: give Claude Code the same task four ways — naked, with a hand-crafted prompt, with our auto-generated prompt, and with a different shard format. All had to make 8 failing tests pass in a 270k-line codebase. Same model. Same starting point.

Here's what happened.

---

## The setup

**Codebase:** Django 5.0.6 — about 270,000 lines of Python across 6,600 files.

**Task:** Eight tests were failing. They expected a model called `EmailChangeRecord` that didn't exist yet. The tests showed *what* the model should do, but gave no hints about *how* to build it.

```python
def test_change_is_recorded(self):
    from change_tracking.models import EmailChangeRecord
    user = User.objects.create_user('alice', email='alice@old.com', password='pass')
    user.email = 'alice@new.com'
    user.save()
    self.assertEqual(EmailChangeRecord.objects.filter(user=user).count(), 1)
```

**What Supermodel added:** Before the test, we ran `supermodel analyze` on the repo. That created a small summary file next to every source file — who calls what, what each module exports, how things connect. A `CLAUDE.md` told Claude to read those summaries first.

No plugins. No special AI tools. Just better context up front.

---

## Results

|                     | Naked Claude | + Supermodel (crafted) | + Supermodel (auto) | Three-file shards |
|---------------------|-------------|------------------------|---------------------|-------------------|
| **Cost**            | $0.30       | $0.12                  | $0.15               | $0.25             |
| **Turns**           | 20          | 9                      | 11                  | 16                |
| **Duration**        | 122s        | 29s                    | 42s                 | 73s               |
| **Tests passed**    | ✓ YES       | ✓ YES                  | ✓ YES               | ✓ YES             |

**60% cheaper. 4× faster. 55% fewer turns.**

All four got the right answer. The only difference was how much digging each one had to do first.

"Crafted" is a hand-written CLAUDE.md with Django-specific hints. "Auto" is what `supermodel skill` generates — a generic prompt that works on any repo. The auto prompt captured 83% of the crafted prompt's savings with zero manual effort.

---

## What actually happened

### Without Supermodel (13 turns, $0.22)

Claude read the tests, then spent 6 turns poking around to figure out how the codebase worked:

```
Bash: run tests → see 8 errors
Read: tests/change_tracking/tests.py
Read: tests/change_tracking/models.py
Bash: ls /app/tests/change_tracking/
Bash: cat __init__.py && test_sqlite.py
Bash: head -50 runtests.py          ← how does INSTALLED_APPS work?
Bash: grep INSTALLED_APPS runtests.py
Bash: sed -n '86,120p' runtests.py  ← find ALWAYS_INSTALLED_APPS
Bash: python -c "import django; print(django.VERSION)"
Write: models.py                    ← finally writes code
Write: apps.py
Bash: run tests → all pass
```

Six commands just to answer basic questions: *How does Django wire things together? Where do signals go? What version is this?* Then it wrote the code.

### With Supermodel (7 turns, $0.13)

```
Bash: run tests → see 8 errors
Read: tests/change_tracking/tests.py
Read: tests/change_tracking/models.py
Glob: tests/change_tracking/**
Write: models.py                    ← writes code immediately
Bash: run tests → all pass
```

No digging. The summary files had already answered the structural questions. Claude went straight to writing.

Here's what Claude said to itself before writing, in each run:

**Without Supermodel** (after 6 exploration turns):
> "Now I understand the structure. I need to implement `EmailChangeRecord` in models.py and wire up signals to track email changes. I'll create an AppConfig to properly connect signals."

**With Supermodel** (before touching anything):
> "I need to implement the `EmailChangeRecord` model and the signal logic to track email changes."

Same conclusion. One of them had to earn it.

---

## Why it's cheaper

Think of tokens like money the AI spends to do its job.

There are two ways to spend tokens: reading files to learn things, and writing files into memory for later. Reading costs more.

The naked run read 235k tokens — mostly source files it combed through to understand the codebase. The Supermodel run read only 90k. That 145k gap is where most of the savings came from.

Here's the twist: the Supermodel run actually *wrote* more tokens (23k vs 19k), because it loaded the summary files into memory upfront. So it spent a little more on the cheap thing. But way less on the expensive thing. Net result: 40% cheaper.

The summary files are built once. When the AI starts working, the answers are already there. It never has to go looking.

---

## Why the task was hard to shortcut

The tests said *what* to build but not *how*. An AI that doesn't already know how Django handles signals has to find out: where does `pre_save`/`post_save` live, how do you catch a field change before it's saved, how does `AppConfig.ready()` work, what does `INSTALLED_APPS` need to include.

That's real exploratory work. The summary files answered all of it before Claude asked a single question.

---

## What this means

The savings didn't come from a cheaper model or a smaller prompt. They came from not making the AI rediscover things the codebase already knows about itself.

On a 270k-line repo with a hard task, one analysis pass meant 11 fewer turns and 93 fewer seconds. And `supermodel skill` generates the CLAUDE.md for you — no hand-tuning required, still 50% cheaper than naked.

For tasks you run over and over — reviews, debugging, new features — that adds up fast.

Run the analysis once. Save on every task after.

---

*Benchmark: identical Docker containers, same model, same task, isolated runs. Full logs in `benchmark/results/`.*
