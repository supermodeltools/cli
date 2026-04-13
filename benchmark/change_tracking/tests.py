from django.test import TestCase
from django.contrib.auth import get_user_model

User = get_user_model()


class EmailChangeTrackingTest(TestCase):

    def test_change_is_recorded(self):
        from change_tracking.models import EmailChangeRecord
        user = User.objects.create_user('alice', email='alice@old.com', password='pass')
        user.email = 'alice@new.com'
        user.save()
        self.assertEqual(EmailChangeRecord.objects.filter(user=user).count(), 1)

    def test_old_email_recorded(self):
        from change_tracking.models import EmailChangeRecord
        user = User.objects.create_user('bob', email='bob@old.com', password='pass')
        user.email = 'bob@new.com'
        user.save()
        self.assertEqual(EmailChangeRecord.objects.get(user=user).old_email, 'bob@old.com')

    def test_new_email_recorded(self):
        from change_tracking.models import EmailChangeRecord
        user = User.objects.create_user('carol', email='carol@old.com', password='pass')
        user.email = 'carol@new.com'
        user.save()
        self.assertEqual(EmailChangeRecord.objects.get(user=user).new_email, 'carol@new.com')

    def test_timestamp_recorded(self):
        from change_tracking.models import EmailChangeRecord
        from django.utils import timezone
        user = User.objects.create_user('dave', email='dave@old.com', password='pass')
        before = timezone.now()
        user.email = 'dave@new.com'
        user.save()
        after = timezone.now()
        ts = EmailChangeRecord.objects.get(user=user).changed_at
        self.assertTrue(before <= ts <= after)

    def test_no_record_on_create(self):
        from change_tracking.models import EmailChangeRecord
        User.objects.create_user('eve', email='eve@example.com', password='pass')
        self.assertEqual(EmailChangeRecord.objects.count(), 0)

    def test_no_record_when_email_unchanged(self):
        from change_tracking.models import EmailChangeRecord
        user = User.objects.create_user('frank', email='frank@example.com', password='pass')
        user.first_name = 'Frank'
        user.save()
        self.assertEqual(EmailChangeRecord.objects.count(), 0)

    def test_multiple_changes_all_recorded(self):
        from change_tracking.models import EmailChangeRecord
        user = User.objects.create_user('grace', email='grace@v1.com', password='pass')
        user.email = 'grace@v2.com'
        user.save()
        user.email = 'grace@v3.com'
        user.save()
        self.assertEqual(EmailChangeRecord.objects.filter(user=user).count(), 2)

    def test_records_deleted_with_user(self):
        from change_tracking.models import EmailChangeRecord
        user = User.objects.create_user('henry', email='henry@old.com', password='pass')
        user.email = 'henry@new.com'
        user.save()
        user.delete()
        self.assertEqual(EmailChangeRecord.objects.count(), 0)
