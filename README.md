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
| POST | /v1/register      | End point for registering a webhook    |
|GET | /v1/webhook/:id  | End point for retrieving a particular webhook |
|PATCH | /v1/webhook     | End point for updating a specific webhook |
|DELETE | /v1/webhook/:id | End point for deleting a particular webhook |


// Personal notes
// I have current taken client id as serial in webhook table. I am little unsure about the table structure right now, but i am going with it. 
// If required I'll change the structure what's the harm.