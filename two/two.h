#ifndef DATADOG_AGENT_SIX_TWO_H
#define DATADOG_AGENT_SIX_TWO_H
#include <vector>
#include <map>
#include <string>

#include <six.h>
#include <Python.h>

class Two: public Six {
public:
    Two(): Six(), _modules() {};
    ~Two();

    // API
    virtual void init(const char* pythonHome);
    virtual void addModuleFunction(const char* module, const char* funcName,
                                   void* func, MethType t);

    // const API
    const char* getPyVersion() const;
    void runAnyFile(const char* path) const;
    SixPyObject* getNone() const { return reinterpret_cast<SixPyObject*>(Py_None); }

private:
    typedef std::vector<PyMethodDef> PyMethods;
    typedef std::map<std::string, PyMethods> PyModules;

    PyModules _modules;
};

#ifdef __cplusplus
extern "C" {
#endif

    Six* create()
    {
        return new Two();
    }

    void destroy(Six* p)
    {
        delete p;
    }

#ifdef __cplusplus
}
#endif
#endif
