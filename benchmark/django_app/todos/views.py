import json
from django.http import JsonResponse
from django.views import View
from django.views.decorators.csrf import csrf_exempt
from django.utils.decorators import method_decorator
from .models import Todo


def todo_to_dict(todo):
    return {
        'id': todo.id,
        'title': todo.title,
        'completed': todo.completed,
        'created_at': todo.created_at.isoformat(),
    }


@method_decorator(csrf_exempt, name='dispatch')
class TodoListView(View):
    def get(self, request):
        todos = Todo.objects.all()
        return JsonResponse({'todos': [todo_to_dict(t) for t in todos]})

    def post(self, request):
        body = json.loads(request.body)
        todo = Todo.objects.create(title=body['title'])
        return JsonResponse(todo_to_dict(todo), status=201)


@method_decorator(csrf_exempt, name='dispatch')
class TodoDetailView(View):
    def get(self, request, pk):
        try:
            todo = Todo.objects.get(pk=pk)
        except Todo.DoesNotExist:
            return JsonResponse({'error': 'Not found'}, status=404)
        return JsonResponse(todo_to_dict(todo))

    def patch(self, request, pk):
        try:
            todo = Todo.objects.get(pk=pk)
        except Todo.DoesNotExist:
            return JsonResponse({'error': 'Not found'}, status=404)
        body = json.loads(request.body)
        if 'completed' in body:
            todo.completed = body['completed']
        todo.save()
        return JsonResponse(todo_to_dict(todo))
