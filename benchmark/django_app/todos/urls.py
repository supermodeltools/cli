from django.urls import path
from . import views

urlpatterns = [
    path('todos/', views.TodoListView.as_view(), name='todo-list'),
    path('todos/<int:pk>/', views.TodoDetailView.as_view(), name='todo-detail'),
]
