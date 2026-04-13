import json
from django.test import TestCase
from .models import Todo


# ── Baseline tests (must pass before and after the feature) ──────────────────

class TodoModelTest(TestCase):
    def test_create_todo(self):
        todo = Todo.objects.create(title='Test todo')
        self.assertEqual(todo.title, 'Test todo')
        self.assertFalse(todo.completed)

    def test_str(self):
        todo = Todo.objects.create(title='Buy milk')
        self.assertEqual(str(todo), 'Buy milk')


class TodoListViewTest(TestCase):
    def test_list_empty(self):
        response = self.client.get('/api/todos/')
        self.assertEqual(response.status_code, 200)
        data = json.loads(response.content)
        self.assertEqual(data['todos'], [])

    def test_list_returns_todos(self):
        Todo.objects.create(title='First')
        Todo.objects.create(title='Second')
        response = self.client.get('/api/todos/')
        self.assertEqual(response.status_code, 200)
        data = json.loads(response.content)
        self.assertEqual(len(data['todos']), 2)

    def test_create_todo(self):
        response = self.client.post(
            '/api/todos/',
            json.dumps({'title': 'New item'}),
            content_type='application/json',
        )
        self.assertEqual(response.status_code, 201)
        data = json.loads(response.content)
        self.assertEqual(data['title'], 'New item')
        self.assertFalse(data['completed'])


class TodoDetailViewTest(TestCase):
    def test_get_todo(self):
        todo = Todo.objects.create(title='Detail test')
        response = self.client.get(f'/api/todos/{todo.id}/')
        self.assertEqual(response.status_code, 200)
        data = json.loads(response.content)
        self.assertEqual(data['title'], 'Detail test')

    def test_get_missing_todo(self):
        response = self.client.get('/api/todos/9999/')
        self.assertEqual(response.status_code, 404)

    def test_patch_complete(self):
        todo = Todo.objects.create(title='Patch me')
        response = self.client.patch(
            f'/api/todos/{todo.id}/',
            json.dumps({'completed': True}),
            content_type='application/json',
        )
        self.assertEqual(response.status_code, 200)
        data = json.loads(response.content)
        self.assertTrue(data['completed'])


# ── Priority feature tests (will FAIL until Claude Code implements them) ──────
#
# Task: Add a `priority` field to Todo with choices: low / medium / high.
# Default priority is "medium". The list endpoint must support filtering via
# ?priority=<value>. The detail endpoint must include `priority` in responses.
# Run `python manage.py makemigrations && python manage.py migrate` after editing.

class PriorityFeatureTest(TestCase):
    def test_priority_field_default(self):
        """New todos default to medium priority."""
        todo = Todo.objects.create(title='Default priority')
        self.assertEqual(todo.priority, 'medium')

    def test_priority_field_high(self):
        """Priority can be set to high."""
        todo = Todo.objects.create(title='Urgent', priority='high')
        self.assertEqual(todo.priority, 'high')

    def test_priority_field_low(self):
        """Priority can be set to low."""
        todo = Todo.objects.create(title='Someday', priority='low')
        self.assertEqual(todo.priority, 'low')

    def test_create_todo_with_priority(self):
        """POST /api/todos/ accepts priority."""
        response = self.client.post(
            '/api/todos/',
            json.dumps({'title': 'Critical task', 'priority': 'high'}),
            content_type='application/json',
        )
        self.assertEqual(response.status_code, 201)
        data = json.loads(response.content)
        self.assertEqual(data['priority'], 'high')

    def test_create_todo_default_priority_in_response(self):
        """POST /api/todos/ returns priority even when not supplied."""
        response = self.client.post(
            '/api/todos/',
            json.dumps({'title': 'Normal task'}),
            content_type='application/json',
        )
        self.assertEqual(response.status_code, 201)
        data = json.loads(response.content)
        self.assertEqual(data['priority'], 'medium')

    def test_list_filter_by_priority(self):
        """GET /api/todos/?priority=high returns only high-priority todos."""
        Todo.objects.create(title='High A', priority='high')
        Todo.objects.create(title='High B', priority='high')
        Todo.objects.create(title='Low C', priority='low')
        response = self.client.get('/api/todos/?priority=high')
        self.assertEqual(response.status_code, 200)
        data = json.loads(response.content)
        self.assertEqual(len(data['todos']), 2)
        for item in data['todos']:
            self.assertEqual(item['priority'], 'high')

    def test_list_no_filter_returns_all(self):
        """GET /api/todos/ without ?priority returns all todos."""
        Todo.objects.create(title='High', priority='high')
        Todo.objects.create(title='Low', priority='low')
        response = self.client.get('/api/todos/')
        self.assertEqual(response.status_code, 200)
        data = json.loads(response.content)
        self.assertEqual(len(data['todos']), 2)

    def test_detail_includes_priority(self):
        """GET /api/todos/<id>/ includes the priority field."""
        todo = Todo.objects.create(title='Check detail', priority='high')
        response = self.client.get(f'/api/todos/{todo.id}/')
        self.assertEqual(response.status_code, 200)
        data = json.loads(response.content)
        self.assertIn('priority', data)
        self.assertEqual(data['priority'], 'high')
