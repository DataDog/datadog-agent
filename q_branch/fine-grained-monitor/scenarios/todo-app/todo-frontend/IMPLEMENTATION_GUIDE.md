# Implementation Guide: todo-frontend

## Quick Start

1. Review the full enhancement prompt in `ENHANCEMENT_PROMPT.md`
2. Implement the required endpoints and logic

## Component Type Guidance

This component uses the **static-app** (frontend) type. The following guidance from the component type template applies:

You are creating a static web application component that serves HTML, CSS, and JavaScript files.

Component: todo-frontend (static-app/frontend)
System: todo-app
Description: React-based frontend for todo application

This component should:
- Serve static files efficiently using Node.js/Express
- Support modern web standards (HTML5, CSS3, ES2022)
- Include responsive design for mobile and desktop
- Provide a clean, modern user interface
- Handle API communication with backend services
- Rely on real container-to-container backend APIs (no mocked/faked/fallback responses); surface clear errors when dependencies are unavailable
- Include proper error handling for network requests
- Support environment-based configuration
- Include browser-based integration tests using Puppeteer

The application will be served by Node.js/Express and should include:
- index.html as the main entry point
- app.js for JavaScript functionality
- style.css for styling
- server.js for Express server configuration
- integration-tests/ directory with Puppeteer-based tests

The integration tests should verify:
- Page loads and renders correctly
- No console errors occur
- UI interactions work as expected
- Form submissions function properly

Focus on creating a production-ready static web application with modern web development practices and comprehensive integration testing.

## Static Resources

If the application needs static images (such as photos of dogs, cats, or food) for display, you can explore and copy resources from `~/dd/gensim/src/gensim-vibecoder/static-resources` as needed. The static-resources directory contains photos organized by category (dogs, cats, food) that can be copied into your component directory and served statically.

*** TASK for `todo-frontend`: You should make sure that the integration tests are comprehensive and working correctly. They should validate the most important use cases / functional requirements.

---

## Checklist

### Endpoints
- [ ] GET /api/todos
- [ ] POST /api/todos
- [ ] PUT /api/todos/{id}
- [ ] DELETE /api/todos/{id}
- [ ] GET /api/todos/{id}
- [ ] PATCH /api/todos/{id}/toggle
- [ ] GET /

### Database Operations

### Downstream Calls
- [ ] Call todo-backend (http.server.request) from list_todos_flow/http.client.request
- [ ] Call todo-backend (http.server.request) from create_todo_flow/http.client.request
- [ ] Call todo-backend (http.server.request) from update_todo_flow/http.client.request
- [ ] Call todo-backend (http.server.request) from delete_todo_flow/http.client.request
- [ ] Call todo-backend (http.server.request) from get_todo_flow/http.client.request
- [ ] Call todo-backend (http.server.request) from toggle_todo_flow/http.client.request

### Telemetry
- [ ] Structured JSON logging with trace correlation
- [ ] Metrics emission (counts, latencies)
- [ ] Distributed tracing spans
- [ ] Health check endpoint

### Testing
- [ ] Docker build succeeds
- [ ] Container starts and passes health check
- [ ] Endpoints return expected responses
- [ ] Logs are emitted
- [ ] No crashes or errors in steady state

### Performance
- [ ] Application responds with reasonable latency

## Key Files to Modify


- `src/html/index.html`: Main HTML page
- `src/html/app.js`: Frontend JavaScript logic
- `src/html/style.css`: Styling
- `src/nginx.conf`: Nginx configuration

## Resources

- **Blueprint**: Full service blueprint specification
- **Enhancement Prompt**: Detailed implementation requirements
- **Environment Variables**: Pre-configured in docker-compose.yml
- **Component Templates**: Base scaffolding in component directory

## Support

For issues with the enhancement process, consult:
1. The full enhancement prompt
2. Flow definitions in the blueprint
3. Component type documentation
4. Example implementations in component-types/
