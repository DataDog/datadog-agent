using Amazon.Lambda.Core;
using System;

[assembly:LambdaSerializer(typeof(Amazon.Lambda.Serialization.SystemTextJson.DefaultLambdaJsonSerializer))]
namespace AwsDotnetCsharp

{
    public class Handler
    {
      public Response Hello()
      {
         return new Response(200, "ok");
      }

      public Response Logs()
      {
        Console.WriteLine("XXX Log 0 XXX");
        Console.WriteLine("XXX Log 1 XXX");
        Console.WriteLine("XXX Log 2 XXX");
        return new Response(200, "ok");
      }
    }
    public class Response
    {
      public int statusCode {get; set;}
      public string body {get; set;}

      public Response(int code, string message){
        statusCode = code;
        body = message;
      }
    }

}