Сюда сборочный скрипт (`build/build.ps1` или `build/build.sh`) кладёт файлы, которые установщик вшивает в себя:

- `GradeStream.exe` — собранная программа из `app/`;
- `cloudflared.exe` — официальный подписанный cloudflared для Windows (скачивается из релизов Cloudflare);
- `gradestream.ico` — иконка (копия `docs/gradestream.ico`).

В репозиторий эти файлы не коммитятся.
