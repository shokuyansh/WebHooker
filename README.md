### WebHooker

Project is on a webhook delivery platform. 

## Features implemented 
* Json Validation of Request and Response 
* Middlewares - Panic recovery
* Database Migrations

## Routes
| Method      | Routes           | Description  |
| ------------- |:------------- | ----- |
| GET      | /v1/healthcheckup | End point for checking health status of server |
| POST | /v1/webhook      | End point for registering a webhook    |
|GET | /v1/webhook/:id  | End point for retrieving a particular webhook |
|PATCH | /v1/webhook/:id     | End point for updating a specific webhook |
|DELETE | /v1/webhook/:id | End point for deleting a particular webhook |
|GET | /v1/webhooks/:id  | List all webhooks under a project |
|GET | /v1/project/:id  | Details of a particular project |
|POST | /v1/project  | Create a new project |


