using Amazon.Lambda.Core;
using Amazon.Lambda.APIGatewayEvents;
using System;
using System.IO;
using System.Collections.Generic;
using System.Threading;
using System.Net;
using System.Net.Http;

[assembly: LambdaSerializer(typeof(Amazon.Lambda.Serialization.SystemTextJson.DefaultLambdaJsonSerializer))]
namespace AwsDotnetCsharp

{
  public class Handler
  {
    public Response Hello()
    {
      return new Response(200, "ok");
    }

    public APIGatewayProxyResponse AppSec()
    {
      return new APIGatewayProxyResponse()
      {
        StatusCode = 200,
        Body = "ok",
        Headers = new Dictionary<string, string>() { { "Content-Type", "text/plain" } }
      };
    }

    public Response Logs()
    {
      // Sleep to ensure correct log ordering
      Thread.Sleep(250);
      Console.WriteLine("XXX Log 0 XXX");
      Thread.Sleep(250);
      Console.WriteLine("XXX Log 1 XXX");
      Thread.Sleep(250);
      Console.WriteLine("XXX Log 2 XXX");
      Thread.Sleep(250);
      return new Response(200, "ok");
    }

    public Response Trace(Request request)
    {
      WebRequest r = WebRequest.Create("https://example.com");
      r.Credentials = CredentialCache.DefaultCredentials;

      HttpWebResponse response = (HttpWebResponse)r.GetResponse();
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
    public int statusCode { get; set; }
    public string body { get; set; }

    public Response(int code, string message)
    {
      statusCode = code;
      body = message;
    }
  }

  public class Request
  {
    public string body { get; set; }
  }
}
