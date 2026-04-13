# Benchmark Results: supermodel vs naked Claude Code
## Setup
- Codebase: django/django @ 5.0.6 (~270k lines)
- Model: claude-sonnet-4-6
- Task: make failing tests in tests/change_tracking/tests.py pass
  (implement EmailChangeRecord — tests give no hints about where to look)

## Results

|                    | naked        | supermodel (crafted) | skill (generic) | three-file   |
|--------------------|--------------|----------------------|-----------------|--------------|
| Cost               | $0.30        | $0.12                | $0.15           | $0.25        |
| Turns              | 20           | 9                    | 11              | 16           |
| Duration           | 122s         | 29s                  | 42s             | 73s          |
| All tests passed   | YES          | YES                  | YES             | YES          |

**supermodel (crafted prompt): 60% cheaper, 76% faster, 55% fewer turns vs naked**
**skill (generic prompt): 50% cheaper, 66% faster, 45% fewer turns vs naked**

## How supermodel helped
The graph files gave Claude the architecture upfront. The supermodel run went straight
to the implementation in 7 turns. The naked run needed 13 turns — 6 extra Bash calls
probing the codebase to figure out where signals live, how User.save() works, and how
to detect field changes before touching any code.

## Files read
naked (2): ['tests/change_tracking/tests.py', 'tests/change_tracking/models.py']
supermodel (2): ['tests/change_tracking/tests.py', 'tests/change_tracking/models.py']
graph files read: none (context injected via hook)
