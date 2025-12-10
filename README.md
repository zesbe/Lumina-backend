# Lumina AI Backend

Go backend API for Lumina AI music generation app.

## Tech Stack
- **Go** 1.23+ with Fiber framework
- **PostgreSQL** 16 database
- **Redis** 7 for caching
- **MiniMax API** for AI music generation
- **Docker** for containerization

## Setup

### 1. Copy environment file
```bash
cp .env.example .env
# Edit .env with your credentials
```

### 2. Run with Docker
```bash
docker-compose up -d
```

### 3. Or run locally
```bash
go mod download
go run cmd/api/main.go
```

## API Endpoints

### Auth
- `POST /api/v1/auth/register` - Register new user
- `POST /api/v1/auth/login` - Login
- `POST /api/v1/auth/refresh` - Refresh token

### Music
- `POST /api/v1/music/generate` - Generate music
- `GET /api/v1/generations` - List user's generations
- `POST /api/v1/generations/:id/favorite` - Toggle favorite
- `POST /api/v1/generations/:id/public` - Toggle public

### Explore (Public)
- `GET /api/v1/explore` - Get public music

## Environment Variables

See `.env.example` for all required variables.

## License
MIT
