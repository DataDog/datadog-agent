using Amazon.Lambda.Core;
using System;
using System.IO;
using System.Collections.Generic;
using System.Threading;
using System.Net;
using System.Net.Http;

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

      public Response Trace()
      {
        WebRequest request = WebRequest.Create("https://example.com");
        request.Credentials = CredentialCache.DefaultCredentials;

        HttpWebResponse response = (HttpWebResponse)request.GetResponse();
        using (Stream dataStream = response.GetResponseStream())
        {
            StreamReader reader = new StreamReader(dataStream);
            string responseFromServer = reader.ReadToEnd();
        }
        response.Close();

        return new Response(200, "ok");
      }

      public Response Timeout()
      {
        Thread.Sleep(100000);

        return new Response(200, "ok");
      }

      public void Error()
      {
        throw new Exception();
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