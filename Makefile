.PHONY: up down doctor doctor-gpu bootstrap test e2e backup restore reset

up:
	docker compose up -d --build --wait

down:
	docker compose down

doctor:
	./scripts/doctor.sh

doctor-gpu:
	./scripts/doctor-gpu.sh

bootstrap:
	./scripts/bootstrap.sh

test:
	cd backend && go test ./...
	cd frontend && corepack pnpm ts:check && corepack pnpm build:prod
	docker compose config --quiet

e2e:
	./scripts/e2e.sh

backup:
	./scripts/backup.sh

restore:
	./scripts/restore.sh "$(BACKUP)"

reset:
	./scripts/reset.sh
