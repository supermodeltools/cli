# Benchmark Results: supermodel vs naked Claude Code
## Setup
- Codebase: django/django @ 5.0.6 (~270k lines)
- Model: claude-sonnet-4-6
- Task: make failing tests in tests/change_tracking/tests.py pass
  (implement EmailChangeRecord — tests give no hints about where to look)

## Results

|                    | naked        | supermodel   |
|--------------------|--------------|--------------|
| Cost               | $0.2212       | $0.1329       |
| Turns              | 13            | 7             |
| Duration           | 95.9s         | 24.1s          |
| Cache tokens read  | 235,456   | 90,479    |
| Cache tokens built | 18,681    | 23,281    |
| All tests passed   | YES          | YES           |
| Tool calls         | {'Bash': 8, 'Read': 2, 'Write': 2} | {'Bash': 2, 'Read': 2, 'Glob': 1, 'Write': 1} |

**supermodel: $0.0883 (39.9%) cheaper, 6 fewer turns, 72s faster**

## How supermodel helped
The graph files gave Claude the architecture upfront. The supermodel run went straight
to the implementation in 7 turns. The naked run needed 13 turns — 6 extra Bash calls
probing the codebase to figure out where signals live, how User.save() works, and how
to detect field changes before touching any code.

## Files read
naked (2): ['tests/change_tracking/tests.py', 'tests/change_tracking/models.py']
supermodel (2): ['tests/change_tracking/tests.py', 'tests/change_tracking/models.py']
graph files read: none (context injected via hook)
